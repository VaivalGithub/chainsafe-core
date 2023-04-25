// Copyright 2021 ChainSafe Systems
// SPDX-License-Identifier: LGPL-3.0-only

package evm

import (
	"context"
	"encoding/json"
	"fmt"
	"math/big"
	"net/http"
	"strings"

	secp256k1 "github.com/ethereum/go-ethereum/crypto"

	"github.com/VaivalGithub/chainsafe-core/chains/evm/calls/consts"
	"github.com/VaivalGithub/chainsafe-core/chains/evm/calls/evmclient"
	"github.com/VaivalGithub/chainsafe-core/chains/evm/calls/evmtransaction"
	"github.com/VaivalGithub/chainsafe-core/chains/evm/calls/transactor"
	"github.com/VaivalGithub/chainsafe-core/chains/evm/executor"
	"github.com/VaivalGithub/chainsafe-core/config/chain"
	"github.com/VaivalGithub/chainsafe-core/relayer/message"
	"github.com/VaivalGithub/chainsafe-core/store"
	"github.com/ethereum/go-ethereum"
	"github.com/ethereum/go-ethereum/accounts/abi"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/ethclient"
	"github.com/rs/zerolog/log"

	"github.com/VaivalGithub/chainsafe-core/e2e/dummy"

	"github.com/VaivalGithub/chainsafe-core/chains/evm/calls/transactor/signAndSend"

	"github.com/VaivalGithub/chainsafe-core/chains/evm/calls/contracts/bridge"
)

type EventListener interface {
	ListenToEvents(ctx context.Context, startBlock *big.Int, msgChan chan *message.Message, errChan chan<- error)
}

type ProposalExecutor interface {
	Execute(message *message.Message, opts transactor.TransactOptions) error
	// FeeClaimByRelayer(p *message.Message) error
	// IsFeeThresholdReached() bool
}

// EVMChain is struct that aggregates all data required for interacting with target chains.
type EVMChain struct {
	listener   EventListener
	writer     ProposalExecutor
	blockstore *store.BlockStore
	config     *chain.EVMConfig
}

func NewEVMChain(listener EventListener, writer ProposalExecutor, blockstore *store.BlockStore, config *chain.EVMConfig) *EVMChain {
	fmt.Printf("Initialising EVM Chain...")
	fmt.Printf("Passed Config: [%+v\n]", config)
	return &EVMChain{listener: listener, writer: writer, blockstore: blockstore, config: config}
}

// PollEvents is the goroutine that polls blocks and searches Deposit events in them.
// Events are then sent to eventsChan.
func (c *EVMChain) PollEvents(ctx context.Context, sysErr chan<- error, msgChan chan *message.Message) {
	log.Info().Msg("Polling Blocks...")

	startBlock, err := c.blockstore.GetStartBlock(
		*c.config.GeneralChainConfig.Id,
		c.config.StartBlock,
		c.config.GeneralChainConfig.LatestBlock,
		c.config.GeneralChainConfig.FreshStart,
	)
	if err != nil {
		sysErr <- fmt.Errorf("error %w on getting last stored block", err)
		return
	}

	go c.listener.ListenToEvents(ctx, startBlock, msgChan, sysErr)
}

func (c *EVMChain) Write(msg *message.Message) error {
	/*
		GasPrice and GasLimit need to be dynamically calculated
		For this we need a method that returns the gas prices for all chains dynamically.
	*/
	fmt.Printf("\nCalculating GasPrice and GasLimit...\n")
	chainProvider, err := ethclient.Dial(c.config.GeneralChainConfig.Endpoint)
	if err != nil {
		fmt.Println("\nFailed to create HTTP provider, resorting to default values:", err)

	} else {
		// Create a new HTTP request
		req, err := http.NewRequest("GET", c.config.GeneralChainConfig.EgsApi, nil)
		if err != nil {
			fmt.Println("\nError creating HTTP request for fetching gas:", err)
		}
		gasProvider := http.DefaultClient
		resp, err := gasProvider.Do(req)
		if err != nil {
			fmt.Println("\nError fetching gas:", err)
		}
		defer resp.Body.Close()
		var dataJson map[string]interface{}
		err = json.NewDecoder(resp.Body).Decode(&dataJson)
		if err != nil {
			fmt.Println("\nError decoding JSON response:", err)
		}
		// Next we fetch the maxFeePerGas and maxPriorityFeePerGas from the JSON
		fastGas := dataJson["fast"].(map[string]interface{})
		maxFastGas := fastGas["maxFee"].(float64)
		maxPriorityGas := fastGas["maxPriorityFee"].(float64)
		fmt.Println("Max Fast Gas:", maxFastGas)
		fmt.Println("Max Priority Gas:", maxPriorityGas)
		maxFastGasWei := maxFastGas * 1000000000
		maxFeePerGas := big.NewInt(int64(maxFastGasWei))
		// Estimating gasLimit
		fromAddress := common.HexToAddress(c.config.GeneralChainConfig.From)
		toAddress := common.HexToAddress(c.config.Bridge)
		bridgeABI, err := abi.JSON(strings.NewReader(consts.BridgeABI))
		if err != nil {
			fmt.Println("\nError parsng Bridge ABI:", err)
		}
		privateKey, err := secp256k1.HexToECDSA(c.config.GeneralChainConfig.Key)
		if err != nil {
			panic(err)
		}
		client, err := evmclient.NewEVMClient(c.config.GeneralChainConfig.Endpoint, privateKey)
		if err != nil {
			panic(err)
		}
		dummyGasPricer := dummy.NewStaticGasPriceDeterminant(client, nil)
		t := signAndSend.NewSignAndSendTransactor(evmtransaction.NewTransaction, dummyGasPricer, client)
		bridgeContract := bridge.NewBridgeContract(client, common.HexToAddress(c.config.Bridge), t)
		fmt.Println("Message Payload:", msg)
		mh := executor.NewEVMMessageHandler(bridgeContract)
		mh.RegisterMessageHandler(c.config.Erc20Handler, executor.ERC20MessageHandler)
		mh.RegisterMessageHandler(c.config.Erc721Handler, executor.ERC721MessageHandler)
		mh.RegisterMessageHandler(c.config.GenericHandler, executor.GenericMessageHandler)
		proposal, err := mh.HandleMessage(msg)
		encodedPayload, err := bridgeABI.Pack("voteProposal", proposal.Source, proposal.DepositNonce, proposal.ResourceId, proposal.Data)
		if err != nil {
			fmt.Println("\nError Encoding Calldata:", err)
		}
		value := big.NewInt(0)
		fmt.Printf("\nFrom: %+v \n To: %+v \n Data: %+v \n Value: %+v \n GasPrice: %+v", fromAddress, &toAddress, encodedPayload, value, maxFeePerGas)
		estimatedGas, err := chainProvider.EstimateGas(context.Background(), ethereum.CallMsg{
			From:     fromAddress,
			To:       &toAddress,
			Data:     encodedPayload,
			Value:    value,
			GasPrice: maxFeePerGas,
		})
		if err != nil {
			fmt.Println("\nError while estimating Gas:", err)
		}
		fmt.Printf("\nGas Limit: [%+v], Gas Price: [%+v]\n", estimatedGas, maxFeePerGas)
		return c.writer.Execute(msg, transactor.TransactOptions{
			GasLimit: estimatedGas,
			GasPrice: maxFeePerGas,
		})
	}
	// the EVMChain contains the config. Let's log it.
	fmt.Printf("\nDefault Config for VoteProposal: [%+v]\n", c.config)
	return c.writer.Execute(msg, transactor.TransactOptions{
		GasLimit: c.config.GasLimit.Uint64(),
		GasPrice: c.config.MaxGasPrice,
	})
}

func (c *EVMChain) DomainID() uint8 {
	return *c.config.GeneralChainConfig.Id
}

// func (c *EVMChain) CheckFeeClaim() bool {
// 	return c.writer.IsFeeThresholdReached()
// }
// func (c *EVMChain) GetFeeClaim(msg *message.Message) error {
// 	return c.writer.FeeClaimByRelayer(msg)
// }
