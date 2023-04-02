// Copyright 2021 ChainSafe Systems
// SPDX-License-Identifier: LGPL-3.0-only

package evm

import (
	"context"
	"fmt"
	"math/big"

	"github.com/VaivalGithub/chainsafe-core/config/chain"
	"github.com/VaivalGithub/chainsafe-core/relayer/message"
	"github.com/VaivalGithub/chainsafe-core/store"
	"github.com/rs/zerolog/log"
)

type EventListener interface {
	ListenToEvents(ctx context.Context, startBlock *big.Int, msgChan chan *message.Message, errChan chan<- error)
}

type ProposalExecutor interface {
	Execute(message *message.Message) error
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
	// the EVMChain contains the config. Let's log it.
	fmt.Printf("\nChain Config for VoteProposal: [%+v]\n", c.config)
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
	return c.writer.Execute(msg)
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
