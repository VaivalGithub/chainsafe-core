// Copyright 2021 ChainSafe Systems
// SPDX-License-Identifier: LGPL-3.0-only

package evm

import (
	"context"
	"fmt"
	"math/big"

	"encoding/json"
	"net/http"

	"github.com/VaivalGithub/chainsafe-core/chains/evm/calls/transactor"
	"github.com/VaivalGithub/chainsafe-core/config/chain"
	"github.com/VaivalGithub/chainsafe-core/relayer/message"
	"github.com/VaivalGithub/chainsafe-core/store"
	"github.com/ethereum/go-ethereum/ethclient"
	"github.com/rs/zerolog/log"
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
	// fmt.Printf("This is a debug message. Did someone trigger VoteProposal?")

	/*
		TransactorOptions interface for reference
		type TransactOptions struct {
			GasLimit uint64
			GasPrice *big.Int
			Value    *big.Int
			Nonce    *big.Int
			ChainID  *big.Int
			Priority uint8
		}
	*/

	/*
		GasPrice and GasLimit need to be dynamically calculated
		For this we need a method that returns the gas prices for all chains dynamically.
	*/
	fmt.Printf("\nCalculating GasPrice and GasLimit...\n")
	_, err := ethclient.Dial(c.config.GeneralChainConfig.Endpoint)
	if err != nil {
		fmt.Errorf("\nFailed to create HTTP provider, resorting to default values:", err)

	} else {
		// Create a new HTTP request
		req, err := http.NewRequest("GET", c.config.GeneralChainConfig.EgsApi, nil)
		if err != nil {
			fmt.Errorf("\nError creating HTTP request for fetching gas:", err)
		}
		// Send the HTTP request and get the response
		gasProvider := http.DefaultClient
		resp, err := gasProvider.Do(req)
		if err != nil {
			fmt.Errorf("\nError fetching gas:", err)
		}
		defer resp.Body.Close()
		// Parse the JSON response body
		var dataJson map[string]interface{}
		err = json.NewDecoder(resp.Body).Decode(&dataJson)
		if err != nil {
			fmt.Errorf("\nError decoding JSON response:", err)
		}
		// Get the "fast" gas price from the JSON data
		fastGas := dataJson["fast"].(map[string]interface{})
		maxFastGas := fastGas["maxFee"].(float64)
		fmt.Println("Max Fast Gas:", maxFastGas)
		maxFastGasWei := maxFastGas * 1000000000
		// Execute Txn with new gas fees
		// We are not passing the gas price for now
		return c.writer.Execute(msg, transactor.TransactOptions{
			GasLimit: uint64(maxFastGasWei),
			GasPrice: c.config.MaxGasPrice,
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
