package chain

import (
	"github.com/syndtr/goleveldb/leveldb"
	"github.com/vitelabs/go-vite/common/types"
	"github.com/vitelabs/go-vite/ledger"
)

// TODO modify account block meta
func (c *Chain) InsertSnapshotBlock(snapshotBlock *ledger.SnapshotBlock) error {
	batch := new(leveldb.Batch)

	// Save snapshot block
	if err := c.chainDb.Sc.WriteSnapshotBlock(batch, snapshotBlock); err != nil {
		c.log.Error("WriteSnapshotBlock failed, error is "+err.Error(), "method", "InsertSnapshotBlock")
		return err
	}

	// Save snapshot content
	if err := c.chainDb.Sc.WriteSnapshotContent(batch, snapshotBlock.Height, snapshotBlock.SnapshotContent); err != nil {
		c.log.Error("WriteSnapshotContent failed, error is "+err.Error(), "method", "InsertSnapshotBlock")
		return err
	}

	// Save snapshot hash index
	c.chainDb.Sc.WriteSnapshotHash(batch, &snapshotBlock.Hash, snapshotBlock.Height)

	// Check and create account
	address := types.PubkeyToAddress(snapshotBlock.PublicKey)
	account, getErr := c.chainDb.Account.GetAccountByAddress(&address)

	if getErr != nil {
		c.log.Error("GetAccountByAddress failed, error is "+getErr.Error(), "method", "InsertSnapshotBlock")
		return getErr
	}

	if account == nil {
		// Create account
		c.createAccountLock.Lock()
		defer c.createAccountLock.Unlock()

		accountId, newAccountIdErr := c.newAccountId()
		if newAccountIdErr != nil {
			c.log.Error("newAccountId failed, error is "+newAccountIdErr.Error(), "method", "InsertSnapshotBlock")
		}

		if err := c.createAccount(batch, accountId, &address, snapshotBlock.PublicKey); err != nil {
			c.log.Error("createAccount failed, error is "+getErr.Error(), "method", "InsertSnapshotBlock")
			return err
		}
	}

	// Write db
	if err := c.chainDb.Commit(batch); err != nil {
		c.log.Error("c.chainDb.Commit(batch) failed, error is "+err.Error(), "method", "InsertSnapshotBlock")
		return err
	}

	// Delete needSnapshotCache
	for addr, item := range snapshotBlock.SnapshotContent {
		c.needSnapshotCache.Remove(&addr, item.AccountBlockHeight)
	}

	return nil
}
func (c *Chain) GetSnapshotBlocksByHash(originBlockHash *types.Hash, count uint64, forward, containSnapshotContent bool) ([]*ledger.SnapshotBlock, error) {
	block, gsErr := c.GetSnapshotBlockByHash(originBlockHash)
	if gsErr != nil {
		c.log.Error("GetSnapshotBlockByHash failed, error is "+gsErr.Error(), "method", "GetSnapshotBlocks")
		return nil, gsErr
	}
	if block == nil {
		return nil, nil
	}

	return c.GetSnapshotBlocksByHeight(block.Height, count, forward, containSnapshotContent)
}

func (c *Chain) GetSnapshotBlocksByHeight(height uint64, count uint64, forward, containSnapshotContent bool) ([]*ledger.SnapshotBlock, error) {
	blocks, gErr := c.chainDb.Sc.GetSnapshotBlocks(height, count, forward, containSnapshotContent)
	if gErr != nil {
		c.log.Error("GetSnapshotBlocks failed, error is "+gErr.Error(), "method", "GetSnapshotBlocksByHeight")
		return nil, gErr
	}
	return blocks, gErr
}

func (c *Chain) GetSnapshotBlockByHeight(height uint64) (*ledger.SnapshotBlock, error) {
	block, gsbErr := c.chainDb.Sc.GetSnapshotBlock(height)
	if gsbErr != nil {
		c.log.Error("GetSnapshotBlock failed, error is "+gsbErr.Error(), "method", "GetSnapshotBlockByHeight")
		return nil, gsbErr
	}

	if block != nil {
		snapshotContent, err := c.chainDb.Sc.GetSnapshotContent(&block.SnapshotHash)
		if err != nil {
			c.log.Error("GetSnapshotContent failed, error is "+err.Error(), "method", "GetSnapshotBlockByHeight")
			return nil, err
		}

		block.SnapshotContent = snapshotContent
	}

	return block, nil
}

func (c *Chain) GetSnapshotBlockByHash(hash *types.Hash) (*ledger.SnapshotBlock, error) {
	height, err := c.chainDb.Sc.GetSnapshotBlockHeight(hash)
	if err != nil {
		c.log.Error("GetSnapshotBlockHeight failed, error is "+err.Error(), "method", "GetSnapshotBlockByHash")
		return nil, err
	}
	if height <= 0 {
		return nil, nil
	}

	return c.GetSnapshotBlockByHeight(height)
}

func (c *Chain) GetLatestSnapshotBlock() (*ledger.SnapshotBlock, error) {

	block, err := c.chainDb.Sc.GetLatestBlock()
	if err != nil {
		c.log.Error("GetLatestBlock failed, error is "+err.Error(), "method", "GetLatestSnapshotBlock")
		return nil, &types.GetError{
			Code: 1,
			Err:  err,
		}
	}

	if block != nil {
		snapshotContent, err := c.chainDb.Sc.GetSnapshotContent(&block.SnapshotHash)
		if err != nil {
			c.log.Error("GetSnapshotContent failed, error is "+err.Error(), "method", "GetLatestSnapshotBlock")
			return nil, &types.GetError{
				Code: 2,
				Err:  err,
			}
		}

		block.SnapshotContent = snapshotContent
	}

	return block, nil
}

func (c *Chain) GetGenesesSnapshotBlock() (*ledger.SnapshotBlock, error) {
	block, err := c.chainDb.Sc.GetGenesesBlock()
	if err != nil {
		c.log.Error("GetGenesesBlock failed, error is "+err.Error(), "method", "GetGenesesSnapshotBlock")
		return nil, &types.GetError{
			Code: 1,
			Err:  err,
		}
	}

	if block != nil {
		snapshotContent, err := c.chainDb.Sc.GetSnapshotContent(&block.SnapshotHash)
		if err != nil {
			c.log.Error("GetSnapshotContent failed, error is "+err.Error(), "method", "GetGenesesSnapshotBlock")
			return nil, &types.GetError{
				Code: 2,
				Err:  err,
			}
		}

		block.SnapshotContent = snapshotContent
	}

	return block, nil
}

func (c *Chain) GetSbHashList(originBlockHash *types.Hash, count, step int, forward bool) ([]*types.Hash, error) {
	height, err := c.chainDb.Sc.GetSnapshotBlockHeight(originBlockHash)
	if err != nil {
		c.log.Error("GetSnapshotBlockHeight failed, error is "+err.Error(), "method", "GetSbHashList")
		return nil, &types.GetError{
			Code: 1,
			Err:  err,
		}
	}

	if height <= 0 {
		return nil, nil
	}

	return c.chainDb.Sc.GetSbHashList(height, count, step, forward), nil
}

func (c *Chain) GetConfirmBlock(accountBlock *ledger.AccountBlock) *ledger.SnapshotBlock {
	height, ghErr := c.chainDb.Ac.GetConfirmHeight(accountBlock)
	if ghErr != nil {
		c.log.Error("GetConfirmHeight failed, error is "+ghErr.Error(), "method", "GetConfirmBlock")
		return nil
	}

	snapshotBlock, gsErr := c.chainDb.Sc.GetSnapshotBlock(height)
	if gsErr != nil {
		c.log.Error("GetSnapshotBlock failed, error is "+ghErr.Error(), "method", "GetConfirmBlock")
		return nil
	}

	return snapshotBlock
}

func (c *Chain) GetConfirmTimes(accountBlock *ledger.AccountBlock) uint64 {
	height, ghErr := c.chainDb.Ac.GetConfirmHeight(accountBlock)
	if ghErr != nil {
		c.log.Error("GetConfirmHeight failed, error is "+ghErr.Error(), "method", "GetConfirmTimes")
		return 0
	}

	if height <= 0 {
		return 0
	}
	latestBlock, latestErr := c.GetLatestSnapshotBlock()
	if latestErr != nil {
		c.log.Error("GetLatestSnapshotBlock failed, error is "+latestErr.Error(), "method", "GetConfirmTimes")
		return 0
	}

	if latestBlock == nil {
		c.log.Error("latestBlock is nil, error is "+latestErr.Error(), "method", "GetConfirmTimes")
		return 0
	}

	return latestBlock.Height - height + 1
}

func (c *Chain) GetConfirmAccountBlock(snapshotHeight uint64, address *types.Address) (*ledger.AccountBlock, error) {
	account, getAccountIdErr := c.chainDb.Account.GetAccountByAddress(address)
	if getAccountIdErr != nil {
		c.log.Error("GetAccountByAddress failed, error is "+getAccountIdErr.Error(), "method", "GetConfirmAccountBlock")
		return nil, types.GetError{
			Code: 1,
			Err:  getAccountIdErr,
		}
	}
	if account == nil {
		return nil, nil
	}

	accountBlock, err := c.chainDb.Ac.GetConfirmAccountBlock(snapshotHeight, account.AccountId)
	if err != nil {
		c.log.Error("GetConfirmAccountBlock failed, error is "+err.Error(), "method", "GetConfirmAccountBlock")
		return nil, types.GetError{
			Code: 2,
			Err:  err,
		}
	}
	return accountBlock, nil
}

// TODO rebuild need_snapshot_cache
// Block only contains hash and height
func (c *Chain) DeleteSnapshotBlocksByHeight(toHeight uint64) ([]*ledger.SnapshotBlock, []*ledger.AccountBlock, error) {
	maxAccountId, err := c.chainDb.Account.GetLastAccountId()
	if err != nil {
		c.log.Error("GetLastAccountId failed, error is "+err.Error(), "method", "DeleteSnapshotBlocksByHeight")
		return nil, nil, err
	}

	planToDelete, getPlanErr := c.chainDb.Ac.GetPlanToDelete(maxAccountId, toHeight)
	if getPlanErr != nil {
		c.log.Error("GetPlanToDelete failed, error is "+getPlanErr.Error(), "method", "DeleteSnapshotBlocksByHeight")
	}

	deleteMap, reopenList, getDeleteAndReopenErr := c.chainDb.Ac.GetDeleteMapAndReopenList(planToDelete)
	if getDeleteAndReopenErr != nil {
		c.log.Error("GetDeleteMapAndReopenList failed, error is "+getDeleteAndReopenErr.Error(), "method", "DeleteSnapshotBlocks")
		return nil, nil, getDeleteAndReopenErr
	}

	batch := new(leveldb.Batch)
	deleteSnapshotBlocks, deleteSnapshotBlocksErr := c.chainDb.Sc.DeleteByHeight(batch, toHeight)
	if deleteSnapshotBlocksErr != nil {
		c.log.Error("DeleteByHeight failed, error is "+deleteSnapshotBlocksErr.Error(), "method", "DeleteSnapshotBlocks")
		return nil, nil, deleteSnapshotBlocksErr
	}

	deleteAccountBlocks, deleteAccountBlocksErr := c.chainDb.Ac.Delete(batch, deleteMap)
	if deleteAccountBlocksErr != nil {
		c.log.Error("Delete failed, error is "+deleteAccountBlocksErr.Error(), "method", "DeleteSnapshotBlocks")
		return nil, nil, deleteAccountBlocksErr
	}

	reopenErr := c.chainDb.Ac.ReopenSendBlocks(batch, reopenList, deleteMap)
	if reopenErr != nil {
		c.log.Error("ReopenSendBlocks failed, error is "+reopenErr.Error(), "method", "DeleteSnapshotBlocks")
		return nil, nil, reopenErr
	}

	writeErr := c.chainDb.Commit(batch)
	if writeErr != nil {
		c.log.Error("Write db failed, error is "+writeErr.Error(), "method", "DeleteSnapshotBlocks")
		return nil, nil, writeErr
	}

	return deleteSnapshotBlocks, deleteAccountBlocks, nil
}