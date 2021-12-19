package tangle

import (
	"fmt"
	"github.com/iotaledger/goshimmer/packages/clock"
	"github.com/iotaledger/goshimmer/packages/tangle/payload"
	"github.com/iotaledger/hive.go/events"
	"math"
	"math/rand"
	"sort"
	"time"
)

const (
	MaxAdversaryTipPoolSize = 2000
	MaxHonestTipPoolSize    = 100
)

type TipManagerOrphanageAttack struct {
	*TipManager
	orderedAdversaryTips []MessageID
	orderedHonestTips    []MessageID
	timestampMap         map[MessageID]time.Time
}

// NewTipManagerOrphanageAttack creates a new tip-selector.
func NewTipManagerOrphanageAttack(legitManager *TipManager) *TipManagerOrphanageAttack {
	maliciousManager := &TipManagerOrphanageAttack{
		TipManager:           legitManager,
		orderedAdversaryTips: make([]MessageID, 0, 100),
		orderedHonestTips:    make([]MessageID, 0, 100),
		timestampMap:         make(map[MessageID]time.Time),
	}

	return maliciousManager
}

func (t *TipManagerOrphanageAttack) Tips(p payload.Payload, countParents int) (parents MessageIDs, err error) {
	if countParents > t.tangle.Options.TipManagerParams.MaxParentsCount {
		countParents = t.tangle.Options.TipManagerParams.MaxParentsCount
	}
	if countParents < t.tangle.Options.TipManagerParams.MinParentsCount {
		countParents = t.tangle.Options.TipManagerParams.MinParentsCount
	}

	parents = t.SelectTips(p, countParents)
	return
}

// Setup sets up the behavior of the component by making it attach to the relevant events of other components.
func (t *TipManagerOrphanageAttack) Setup() {
	t.tangle.Orderer.Events.MessageOrdered.Attach(events.NewClosure(func(messageID MessageID) {
		t.tangle.Storage.Message(messageID).Consume(t.AddTip)
	}))

	t.events.TipRemoved.Attach(events.NewClosure(func(tipEvent *TipEvent) {
		t.tipsCleaner.Cancel(tipEvent.MessageID)
	}))

	MaxParentsCount = t.tangle.Options.TipManagerParams.MaxParentsCount
	MinParentsCount = t.tangle.Options.TipManagerParams.MinParentsCount
}

func (t *TipManagerOrphanageAttack) AddTip(message *Message) {
	tipLifeGracePeriod := t.tangle.Options.SolidifierParams.MaxParentsTimeDifference - t.tangle.Options.TipManagerParams.TipLifeGracePeriodDiff

	timestamp := message.issuingTime

	messageID := message.ID()
	cachedMessageMetadata := t.tangle.Storage.MessageMetadata(messageID)
	messageMetadata := cachedMessageMetadata.Unwrap()
	defer cachedMessageMetadata.Release()

	if messageMetadata == nil {
		panic(fmt.Errorf("failed to load MessageMetadata with %s", messageID))
	}

	if clock.Since(message.IssuingTime()) > tipLifeGracePeriod {
		return
	}

	var tipSet *[]MessageID
	if message.IssuerPublicKey() == t.tangle.Options.Identity.PublicKey() {
		tipSet = &t.orderedAdversaryTips
		*tipSet = t.insertTip(*tipSet, messageID, timestamp, MaxAdversaryTipPoolSize)
	} else {
		tipSet = &t.orderedHonestTips
		*tipSet = t.insertTip(*tipSet, messageID, timestamp, MaxHonestTipPoolSize)
	}

	t.events.TipAdded.Trigger(&TipEvent{
		MessageID: messageID,
	})

	t.tipsCleaner.ExecuteAt(messageID, func() {
		// delete the oldest tip from the tip set
		correctTipSet := tipSet
		if len(*correctTipSet) > 0 {
			*correctTipSet = (*correctTipSet)[:len(*correctTipSet)-1]
		}
	}, message.IssuingTime().Add(tipLifeGracePeriod))
}

func (t *TipManagerOrphanageAttack) SelectTips(p payload.Payload, count int) (parents MessageIDs) {
	parents = make([]MessageID, 0, t.tangle.Options.TipManagerParams.MaxParentsCount)

	parents = append(parents, t.getTips(count, t.orderedAdversaryTips)...)
	if len(parents) >= t.tangle.Options.TipManagerParams.MinParentsCount {
		// minimum number of tips selected
		return
	}
	// fill with honest tips up to min required number of tips
	tipsLeft := t.tangle.Options.TipManagerParams.MinParentsCount - len(parents)
	parents = append(parents, t.getTips(tipsLeft, t.orderedHonestTips)...)

	return
}

func (t *TipManagerOrphanageAttack) getTips(parentCount int, tipSet []MessageID) []MessageID {
	if len(tipSet) > parentCount {
		return tipSet[len(tipSet)-parentCount:]
	}
	return tipSet[:]
}

// insertTip add tip to the malicious tip set and keeps descending order, so the oldest tips will be at the end
func (t *TipManagerOrphanageAttack) insertTip(tipSet []MessageID, id MessageID, timestamp time.Time, maxTipPoolSize int) []MessageID {
	t.timestampMap[id] = timestamp
	idx := sort.Search(len(tipSet), func(idx int) bool {
		return timestamp.UnixNano() >= t.timestampMap[(tipSet)[idx]].UnixNano()
	})
	tipSet = insertTipAt(tipSet, id, idx, maxTipPoolSize)
	return tipSet
}

// TipCount the amount of strong tips.
func (t *TipManagerOrphanageAttack) TipCount() int {
	return len(t.orderedAdversaryTips) + len(t.orderedHonestTips)
}

// AllTips returns a list of all tips that are stored in the TipManger.
func (t *TipManagerOrphanageAttack) AllTips() MessageIDs {
	return append(t.orderedHonestTips, t.orderedAdversaryTips...)
}

// insertTipAt inserts tip at given index
func insertTipAt(tipSet []MessageID, id MessageID, idx, maxTipPoolSize int) []MessageID {
	if idx == len(tipSet) {
		return append(tipSet, id)
	}
	// limit the tip pool size to MaxTipPoolSize
	// by removing random tip with probability that decreases when index increases
	if len(tipSet) > maxTipPoolSize {
		indexToRemove := chooseTipWithDecreasingProb(maxTipPoolSize)
		// remove element at indexToRemove
		tipSet = append(tipSet[:indexToRemove], tipSet[indexToRemove+1:]...)
	}

	// make place for new item at index idx
	tipSet = append(tipSet[:idx+1], tipSet[idx:]...)

	// insert new tip and keep the order
	tipSet[idx] = id
	return tipSet
}

// choose the tip that will be removed with a decreasing probability for higher indexes
func chooseTipWithDecreasingProb(lengthOfArray int) int {
	// last index has probability of being selected equal zero
	lastIndex := float64(lengthOfArray) - 1
	cdf := func(n float64) float64 {
		return n * (n + 1) / 2
	}
	X := rand.Intn(int(cdf(lastIndex)))
	invX := (math.Sqrt(1+8*float64(X)) - 1) / 2
	// with the formulas above the most common are the lowest indexes, we need the opposite
	index := lengthOfArray - 1 - int(invX)
	return index
}
