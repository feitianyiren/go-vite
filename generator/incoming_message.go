package generator

import (
	"github.com/pkg/errors"
	"github.com/vitelabs/go-vite/common/types"
	"github.com/vitelabs/go-vite/ledger"
	"math/big"
	"time"
)

type IncomingMessage struct {
	BlockType byte

	AccountAddress types.Address
	ToAddress      *types.Address
	FromBlockHash  *types.Hash

	TokenId *types.TokenTypeId
	Amount  big.Int
	Nonce   []byte
	Data    []byte
}

func (im *IncomingMessage) ToBlock() (block *ledger.AccountBlock, err error) {
	select {
	case im.BlockType == ledger.BlockTypeSendCall:
		block.BlockType = im.BlockType

		if im.ToAddress != nil {
			block.ToAddress = *im.ToAddress
		} else {
			block.ToAddress = types.Address{}
		}
		block.FromBlockHash = types.Hash{}

		block.Amount = &im.Amount
		block.Nonce = im.Nonce
		block.Data = im.Data

		if im.TokenId != nil {
			block.TokenId = *im.TokenId
		} else {
			return nil, errors.New("BlockTypeSendCall's TokenId can't be nil")
		}

	case im.BlockType == ledger.BlockTypeSendCreate:
		block.BlockType = im.BlockType

		if im.ToAddress != nil {
			block.ToAddress = *im.ToAddress
		} else {
			block.ToAddress = types.Address{}
		}
		block.FromBlockHash = types.Hash{}

		block.Amount = &im.Amount
		block.Nonce = im.Nonce

		if im.Data != nil {
			block.Data = im.Data
		} else {
			return nil, errors.New("BlockTypeSendCreate's Data can't be nil")
		}

		if im.TokenId != nil {
			block.TokenId = *im.TokenId
		} else {
			block.TokenId = types.TokenTypeId{}
		}

	default:
		block.BlockType = ledger.BlockTypeReceive

		block.ToAddress = types.Address{}
		if im.FromBlockHash != nil {
			block.FromBlockHash = *im.FromBlockHash
		} else {
			block.FromBlockHash = types.Hash{}
		}
		block.Amount = &im.Amount
		block.Nonce = im.Nonce
		block.Data = im.Data

		if im.TokenId != nil {
			block.TokenId = *im.TokenId
		} else {
			return nil, errors.New("BlockTypeReceive's TokenId can't be nil")
		}
	}
	return block, err
}

type ConsensusMessage struct {
	SnapshotHash types.Hash
	Timestamp    time.Time
	Producer     types.Address
}
