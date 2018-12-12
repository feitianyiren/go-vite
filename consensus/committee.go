package consensus

import (
	"time"

	"strconv"

	"sync"

	"github.com/pkg/errors"
	"github.com/vitelabs/go-vite/common"
	"github.com/vitelabs/go-vite/common/types"
	"github.com/vitelabs/go-vite/consensus/core"
	"github.com/vitelabs/go-vite/ledger"
	"github.com/vitelabs/go-vite/log15"
)

type subscribeEvent struct {
	addr *types.Address
	gid  types.Gid
	fn   func(Event)
}
type producerSubscribeEvent struct {
	gid types.Gid
	fn  func(ProducersEvent)
}

// update committee result
type committee struct {
	common.LifecycleStatus

	mLog log15.Logger

	genesis  time.Time
	rw       *chainRw
	snapshot *teller
	contract *teller
	tellers  sync.Map
	signer   types.Address

	// subscribes map[types.Gid]map[string]*subscribeEvent
	subscribes sync.Map

	wg     sync.WaitGroup
	closed chan struct{}
}

func (self *committee) VerifySnapshotProducer(header *ledger.SnapshotBlock) (bool, error) {
	tel := self.snapshot
	electionResult, err := tel.electionTime(*header.Timestamp)
	if err != nil {
		return false, err
	}

	return self.verifyProducer(*header.Timestamp, header.Producer(), electionResult), nil
}
func (self *committee) initTeller(gid types.Gid) (*teller, error) {
	info := self.rw.GetMemberInfo(gid, self.genesis)
	if info == nil {
		return nil, errors.New("can't get member info.")
	}
	t := newTeller(info, self.rw, self.mLog)
	self.tellers.Store(gid, t)
	return t, nil
}

func (self *committee) VerifyAccountProducer(header *ledger.AccountBlock) (bool, error) {
	gid, err := self.rw.getGid(header)
	if err != nil {
		return false, err
	}
	t, ok := self.tellers.Load(gid)
	if !ok {
		tmp, err := self.initTeller(gid)
		if err != nil {
			return false, err
		}
		t = tmp
	}
	if t == nil {
		return false, errors.New("consensus group not exist")
	}
	tel := t.(*teller)

	electionResult, err := tel.electionTime(*header.Timestamp)
	if err != nil {
		return false, err
	}

	err = tel.rw.checkSnapshotHashValid(electionResult.Height, electionResult.Hash, header.SnapshotHash)
	if err != nil {
		return false, err
	}
	return self.verifyProducer(*header.Timestamp, header.Producer(), electionResult), nil
}

func (self *committee) verifyProducer(t time.Time, address types.Address, result *electionResult) bool {
	if result == nil {
		return false
	}
	for _, plan := range result.Plans {
		if plan.Member == address {
			if plan.STime == t {
				return true
			}
		}
	}
	return false
}

func (self *committee) ReadByIndex(gid types.Gid, index uint64) ([]*Event, uint64, error) {
	t, ok := self.tellers.Load(gid)
	if !ok {
		tmp, err := self.initTeller(gid)
		if err != nil {
			return nil, 0, err
		}
		t = tmp
	}
	if t == nil {
		return nil, 0, errors.New("consensus group not exist")
	}
	tel := t.(*teller)
	electionResult, err := tel.electionIndex(index)

	if err != nil {
		return nil, 0, err
	}
	var result []*Event
	for _, p := range electionResult.Plans {
		e := newConsensusEvent(electionResult, p, gid)
		result = append(result, &e)
	}
	return result, uint64(electionResult.Index), nil
}

func (self *committee) ReadByTime(gid types.Gid, t2 time.Time) ([]*Event, uint64, error) {
	t, ok := self.tellers.Load(gid)
	if !ok {
		tmp, err := self.initTeller(gid)
		if err != nil {
			return nil, 0, err
		}
		t = tmp
	}
	if t == nil {
		return nil, 0, errors.New("consensus group not exist")
	}
	tel := t.(*teller)
	electionResult, err := tel.electionTime(t2)

	if err != nil {
		return nil, 0, err
	}
	var result []*Event
	for _, p := range electionResult.Plans {
		e := newConsensusEvent(electionResult, p, gid)
		result = append(result, &e)
	}
	return result, uint64(electionResult.Index), nil
}
func (self *committee) ReadVoteMapByTime(gid types.Gid, index uint64) ([]*VoteDetails, *ledger.HashHeight, error) {
	t, ok := self.tellers.Load(gid)
	if !ok {
		tmp, err := self.initTeller(gid)
		if err != nil {
			return nil, nil, err
		}
		t = tmp
	}
	if t == nil {
		return nil, nil, errors.New("consensus group not exist")
	}
	tel := t.(*teller)

	return tel.voteDetails(index)
}

func (self *committee) ReadVoteMapForAPI(gid types.Gid, ti time.Time) ([]*VoteDetails, *ledger.HashHeight, error) {
	t, ok := self.tellers.Load(gid)
	if !ok {
		tmp, err := self.initTeller(gid)
		if err != nil {
			return nil, nil, err
		}
		t = tmp
	}
	if t == nil {
		return nil, nil, errors.New("consensus group not exist")
	}
	tel := t.(*teller)

	return tel.voteDetailsBeforeTime(ti)
}

func (self *committee) VoteTimeToIndex(gid types.Gid, t2 time.Time) (uint64, error) {
	t, ok := self.tellers.Load(gid)
	if !ok {
		tmp, err := self.initTeller(gid)
		if err != nil {
			return 0, err
		}
		t = tmp
	}
	if t == nil {
		return 0, errors.New("consensus group not exist")
	}
	tel := t.(*teller)

	index := tel.time2Index(t2)
	return uint64(index), nil
}

func (self *committee) VoteIndexToTime(gid types.Gid, i uint64) (*time.Time, *time.Time, error) {
	t, ok := self.tellers.Load(gid)
	if !ok {
		tmp, err := self.initTeller(gid)
		if err != nil {
			return nil, nil, err
		}
		t = tmp
	}
	if t == nil {
		return nil, nil, errors.New("consensus group not exist")
	}
	tel := t.(*teller)

	st, et := tel.index2Time(i)
	return &st, &et, nil
}

func NewConsensus(genesisTime time.Time, ch ch) *committee {
	committee := &committee{rw: &chainRw{rw: ch}, genesis: genesisTime}
	committee.mLog = log15.New("module", "consensus/committee")
	return committee
}

func (self *committee) Init() error {
	if !self.PreInit() {
		return errors.New("pre init fail.")
	}
	defer self.PostInit()
	{
		t, err := self.initTeller(types.SNAPSHOT_GID)
		if err != nil {
			return err
		}
		self.snapshot = t
	}
	{
		t, err := self.initTeller(types.DELEGATE_GID)
		if err != nil {
			return err
		}
		self.contract = t
	}
	return nil
}

func (self *committee) Start() {
	self.PreStart()
	defer self.PostStart()
	self.closed = make(chan struct{})

	self.wg.Add(1)
	snapshotSubs, _ := self.subscribes.LoadOrStore(types.SNAPSHOT_GID, &sync.Map{})

	tmpSnapshot := self.snapshot
	common.Go(func() {
		defer self.wg.Done()
		self.update(tmpSnapshot, snapshotSubs.(*sync.Map))
	})

	self.wg.Add(1)
	contractSubs, _ := self.subscribes.LoadOrStore(types.DELEGATE_GID, &sync.Map{})

	tmpContract := self.contract
	common.Go(func() {
		defer self.wg.Done()
		self.update(tmpContract, contractSubs.(*sync.Map))
	})
}

func (self *committee) Stop() {
	self.PreStop()
	defer self.PostStop()

	close(self.closed)
	self.wg.Wait()
}

func (self *committee) Subscribe(gid types.Gid, id string, addr *types.Address, fn func(Event)) {
	value, ok := self.subscribes.Load(gid)
	if !ok {
		value, _ = self.subscribes.LoadOrStore(gid, &sync.Map{})
	}
	v := value.(*sync.Map)
	v.Store(id, &subscribeEvent{addr: addr, fn: fn, gid: gid})
}
func (self *committee) UnSubscribe(gid types.Gid, id string) {
	value, ok := self.subscribes.Load(gid)
	if !ok {
		return
	}
	v := value.(*sync.Map)
	v.Delete(id)
}

func (self *committee) SubscribeProducers(gid types.Gid, id string, fn func(event ProducersEvent)) {
	value, ok := self.subscribes.Load(gid)
	if !ok {
		value, _ = self.subscribes.LoadOrStore(gid, &sync.Map{})
	}
	v := value.(*sync.Map)
	v.Store(id, &producerSubscribeEvent{fn: fn, gid: gid})
}

func (self *committee) update(t *teller, m *sync.Map) {
	index := t.time2Index(time.Now())
	for !self.Stopped() {
		//var current *memberPlan = nil
		electionResult, err := t.electionIndex(index)

		if err != nil {
			self.mLog.Error("can't get election result. time is "+time.Now().Format(time.RFC3339Nano)+"\".", "err", err)
			time.Sleep(time.Second)
			// error handle
			continue
		}

		if electionResult.Index != index {
			self.mLog.Error("can't get Index election result. Index is " + strconv.FormatInt(int64(index), 10))
			index = t.time2Index(time.Now())
			continue
		}
		subs1, subs2 := copyMap(m)

		if len(subs1) == 0 && len(subs2) == 0 {
			select {
			case <-time.After(electionResult.ETime.Sub(time.Now())):
			case <-self.closed:
				return
			}
			index = index + 1
			continue
		}

		for _, v := range subs1 {
			tmpV := v
			tmpResult := electionResult
			common.Go(func() {
				self.event(tmpV, tmpResult)
			})
		}

		for _, v := range subs2 {
			tmpV := v
			tmpResult := electionResult
			common.Go(func() {
				self.eventProducer(tmpV, tmpResult)
			})
		}

		sleepT := electionResult.ETime.Sub(time.Now()) - time.Millisecond*500
		select {
		case <-time.After(sleepT):
		case <-self.closed:
			return
		}
		index = electionResult.Index + 1
	}
}
func copyMap(m *sync.Map) (map[string]*subscribeEvent, map[string]*producerSubscribeEvent) {
	r1 := make(map[string]*subscribeEvent)
	r2 := make(map[string]*producerSubscribeEvent)
	m.Range(func(k, v interface{}) bool {
		switch t := v.(type) {
		case *subscribeEvent:
			r1[k.(string)] = t
		case *producerSubscribeEvent:
			r2[k.(string)] = t
		}
		return true
	})
	return r1, r2
}
func (self *committee) eventProducer(e *producerSubscribeEvent, result *electionResult) {
	self.wg.Add(1)
	defer self.wg.Done()
	var r []types.Address
	for _, v := range result.Plans {
		r = append(r, v.Member)
	}
	e.fn(ProducersEvent{Addrs: r, Index: result.Index, Gid: e.gid})
}

func (self *committee) event(e *subscribeEvent, result *electionResult) {
	self.wg.Add(1)
	defer self.wg.Done()
	if e.addr == nil {
		// all
		self.eventAll(e, result)
	} else {
		self.eventAddr(e, result)
	}
}

func (self *committee) eventAll(e *subscribeEvent, result *electionResult) {
	for _, p := range result.Plans {
		now := time.Now()
		sub := p.STime.Sub(now)
		if sub+time.Second < 0 {
			continue
		}

		if sub > time.Millisecond*10 {
			time.Sleep(sub)
		}

		e.fn(newConsensusEvent(result, p, e.gid))
	}
}
func (self *committee) eventAddr(e *subscribeEvent, result *electionResult) {
	for _, p := range result.Plans {
		if p.Member == *e.addr {
			now := time.Now()
			sub := p.STime.Sub(now)
			if sub+time.Second < 0 {
				continue
			}
			if sub > time.Millisecond*10 {
				time.Sleep(sub)
			}
			e.fn(newConsensusEvent(result, p, e.gid))
		}
	}
}

func newConsensusEvent(r *electionResult, p *core.MemberPlan, gid types.Gid) Event {
	return Event{
		Gid:            gid,
		Address:        p.Member,
		Stime:          p.STime,
		Etime:          p.ETime,
		Timestamp:      p.STime,
		SnapshotHash:   r.Hash,
		SnapshotHeight: r.Height,
	}
}
