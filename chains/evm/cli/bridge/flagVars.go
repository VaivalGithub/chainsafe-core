package bridge

import (
	"math/big"

	"github.com/ethereum/go-ethereum/common"
	"github.com/VaivalGithub/chainsafe-core/crypto/secp256k1"
	"github.com/VaivalGithub/chainsafe-core/types"
)

//flag vars
var (
	Bridge          string
	DataHash        string
	DomainID        uint8
	Data            string
	DepositNonce    uint64
	Handler         string
	ResourceID      string
	Target          string
	Deposit         string
	DepositerOffset uint64
	Execute         string
	Hash            bool
	TokenContract   string
)

//processed flag vars
var (
	BridgeAddr         common.Address
	ResourceIdBytesArr types.ResourceID
	HandlerAddr        common.Address
	TargetContractAddr common.Address
	TokenContractAddr  common.Address
	DepositSigBytes    [4]byte
	ExecuteSigBytes    [4]byte
	DataBytes          []byte
)

// global flags
var (
	url           string
	gasLimit      uint64
	gasPrice      *big.Int
	senderKeyPair *secp256k1.Keypair
	prepare       bool
)
