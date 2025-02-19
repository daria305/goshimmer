package tangle

import (
	"fmt"
	"sync"
	"time"

	"github.com/cockroachdb/errors"
	"github.com/iotaledger/hive.go/datastructure/randommap"
	"github.com/iotaledger/hive.go/events"
	"github.com/iotaledger/hive.go/timedexecutor"
	"github.com/iotaledger/hive.go/timedqueue"
	"github.com/iotaledger/hive.go/types"

	"github.com/iotaledger/goshimmer/packages/clock"
	"github.com/iotaledger/goshimmer/packages/ledgerstate"
	"github.com/iotaledger/goshimmer/packages/tangle/payload"
)

// region TimedTaskExecutor ////////////////////////////////////////////////////////////////////////////////////////////

// TimedTaskExecutor is a TimedExecutor that internally manages the scheduled callbacks as tasks with a unique
// identifier. It allows to replace existing scheduled tasks and cancel them using the same identifier.
type TimedTaskExecutor struct {
	*timedexecutor.TimedExecutor
	queuedElements      map[interface{}]*timedqueue.QueueElement
	queuedElementsMutex sync.Mutex
}

// NewTimedTaskExecutor is the constructor of the TimedTaskExecutor.
func NewTimedTaskExecutor(workerCount int) *TimedTaskExecutor {
	return &TimedTaskExecutor{
		TimedExecutor:  timedexecutor.New(workerCount),
		queuedElements: make(map[interface{}]*timedqueue.QueueElement),
	}
}

// ExecuteAfter executes the given function after the given delay.
func (t *TimedTaskExecutor) ExecuteAfter(identifier interface{}, callback func(), delay time.Duration) *timedexecutor.ScheduledTask {
	t.queuedElementsMutex.Lock()
	defer t.queuedElementsMutex.Unlock()

	queuedElement, queuedElementExists := t.queuedElements[identifier]
	if queuedElementExists {
		queuedElement.Cancel()
	}

	t.queuedElements[identifier] = t.TimedExecutor.ExecuteAfter(func() {
		callback()

		t.queuedElementsMutex.Lock()
		defer t.queuedElementsMutex.Unlock()

		delete(t.queuedElements, identifier)
	}, delay)

	return t.queuedElements[identifier]
}

// ExecuteAt executes the given function at the given time.
func (t *TimedTaskExecutor) ExecuteAt(identifier interface{}, callback func(), executionTime time.Time) *timedexecutor.ScheduledTask {
	t.queuedElementsMutex.Lock()
	defer t.queuedElementsMutex.Unlock()

	queuedElement, queuedElementExists := t.queuedElements[identifier]
	if queuedElementExists {
		queuedElement.Cancel()
	}

	t.queuedElements[identifier] = t.TimedExecutor.ExecuteAt(func() {
		callback()

		t.queuedElementsMutex.Lock()
		defer t.queuedElementsMutex.Unlock()

		delete(t.queuedElements, identifier)
	}, executionTime)

	return t.queuedElements[identifier]
}

// Cancel cancels a queued task.
func (t *TimedTaskExecutor) Cancel(identifier interface{}) (canceled bool) {
	t.queuedElementsMutex.Lock()
	defer t.queuedElementsMutex.Unlock()

	queuedElement, queuedElementExists := t.queuedElements[identifier]
	if !queuedElementExists {
		return
	}

	queuedElement.Cancel()
	delete(t.queuedElements, identifier)

	return true
}

// endregion ///////////////////////////////////////////////////////////////////////////////////////////////////////////

// region TipManager ///////////////////////////////////////////////////////////////////////////////////////////////////

// TipManagerParams represents the parameters for the Tip Manager.
type TipManagerParams struct {
	MinParentsCount        int
	MaxParentsCount        int
	TipLifeGracePeriodDiff time.Duration
}

// TipManagerInterface defines the interface for the Tip Manager
type TipManagerInterface interface {
	Setup()
	Set(tips ...MessageID)
	AddTip(message *Message)
	Tips(p payload.Payload, countParents int) (parents MessageIDs, err error)
	SelectTips(p payload.Payload, count int) (parents MessageIDs)
	AllTips() MessageIDs
	TipCount() int
	Shutdown()
	TipSet() *randommap.RandomMap
	Events() *TipManagerEvents
}

// TipManager manages a map of tips and emits events for their removal and addition.
type TipManager struct {
	tangle      *Tangle
	tips        *randommap.RandomMap
	tipsCleaner *TimedTaskExecutor
	events      *TipManagerEvents
}

// NewTipManager creates a new tip-selector.
func NewTipManager(tangle *Tangle, tips ...MessageID) *TipManager {
	tipSelector := &TipManager{
		tangle:      tangle,
		tips:        randommap.New(),
		tipsCleaner: NewTimedTaskExecutor(1),
		events: &TipManagerEvents{
			TipAdded:   events.NewEvent(tipEventHandler),
			TipRemoved: events.NewEvent(tipEventHandler),
		},
	}

	if tips != nil {
		tipSelector.Set(tips...)
	}

	return tipSelector
}

func (t *TipManager) TipSet() *randommap.RandomMap {
	return t.tips
}

func (t *TipManager) Events() *TipManagerEvents {
	return t.events
}

// Setup sets up the behavior of the component by making it attach to the relevant events of other components.
func (t *TipManager) Setup() {
	t.tangle.Orderer.Events.MessageOrdered.Attach(events.NewClosure(func(messageID MessageID) {
		t.tangle.Storage.Message(messageID).Consume(t.AddTip)
	}))

	t.events.TipRemoved.Attach(events.NewClosure(func(tipEvent *TipEvent) {
		t.tipsCleaner.Cancel(tipEvent.MessageID)
	}))

	MaxParentsCount = t.tangle.Options.TipManagerParams.MaxParentsCount
	MinParentsCount = t.tangle.Options.TipManagerParams.MinParentsCount
}

// Set adds the given messageIDs as tips.
func (t *TipManager) Set(tips ...MessageID) {
	for _, messageID := range tips {
		t.tips.Set(messageID, messageID)
	}
}

// AddTip adds the message to the tip pool if its issuing time is within the tipLifeGracePeriod.
// Parents of a message that are currently tip lose the tip status and are removed.
func (t *TipManager) AddTip(message *Message) {
	tipLifeGracePeriod := t.tangle.Options.SolidifierParams.MaxParentsTimeDifference - t.tangle.Options.TipManagerParams.TipLifeGracePeriodDiff

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

	// TODO: possible logical race condition if a child message gets added before its parents.
	//  To be sure we probably need to check "It is not directly referenced by any strong message via strong/weak parent"
	//  before adding a message as a tip. For now we're using only 1 worker after the scheduler and it shouldn't be a problem.

	if t.tips.Set(messageID, messageID) {
		t.events.TipAdded.Trigger(&TipEvent{
			MessageID: messageID,
		})

		t.tipsCleaner.ExecuteAt(messageID, func() {
			t.tips.Delete(messageID)
		}, message.IssuingTime().Add(tipLifeGracePeriod))
	}

	// skip removing tips if TangleWidth is enabled
	if t.TipCount() <= t.tangle.Options.TangleWidth {
		return
	}

	// a tip loses its tip status if it is referenced by another message
	message.ForEachParentByType(StrongParentType, func(parentMessageID MessageID) {
		if _, deleted := t.tips.Delete(parentMessageID); deleted {
			t.events.TipRemoved.Trigger(&TipEvent{
				MessageID: parentMessageID,
			})
		}
	})
}

// Tips returns count number of tips, maximum MaxParentsCount.
func (t *TipManager) Tips(p payload.Payload, countParents int) (parents MessageIDs, err error) {
	if countParents > t.tangle.Options.TipManagerParams.MaxParentsCount {
		countParents = t.tangle.Options.TipManagerParams.MaxParentsCount
	}
	if countParents < t.tangle.Options.TipManagerParams.MinParentsCount {
		countParents = t.tangle.Options.TipManagerParams.MinParentsCount
	}

	// select parents
	parents = t.SelectTips(p, countParents)
	// if transaction, make sure that all inputs are in the past cone of the selected tips
	if p != nil && p.Type() == ledgerstate.TransactionType {
		transaction := p.(*ledgerstate.Transaction)

		tries := 5
		for !t.tangle.Utils.AllTransactionsApprovedByMessages(transaction.ReferencedTransactionIDs(), parents...) {
			if tries == 0 {
				err = errors.Errorf("not able to make sure that all inputs are in the past cone of selected tips")
				return nil, err
			}
			tries--

			parents = t.SelectTips(p, t.tangle.Options.TipManagerParams.MaxParentsCount)
		}
	}

	return
}

// SelectTips returns a list of parents. In case of a transaction, it references young enough attachments
// of consumed transactions directly. Otherwise/additionally count tips are randomly selected.
func (t *TipManager) SelectTips(p payload.Payload, count int) (parents MessageIDs) {
	parents = make([]MessageID, 0, t.tangle.Options.TipManagerParams.MaxParentsCount)
	parentsMap := make(map[MessageID]types.Empty)

	// if transaction: reference young parents directly
	if p != nil && p.Type() == ledgerstate.TransactionType {
		transaction := p.(*ledgerstate.Transaction)

		referencedTransactionIDs := transaction.ReferencedTransactionIDs()
		if len(referencedTransactionIDs) <= 8 {
			for transactionID := range referencedTransactionIDs {
				// only one attachment needs to be added
				added := false

				for _, attachmentMessageID := range t.tangle.Storage.AttachmentMessageIDs(transactionID) {
					t.tangle.Storage.Message(attachmentMessageID).Consume(func(message *Message) {
						// check if message is too old
						timeDifference := clock.SyncedTime().Sub(message.IssuingTime())
						if timeDifference <= t.tangle.Options.SolidifierParams.MaxParentsTimeDifference {
							if _, ok := parentsMap[attachmentMessageID]; !ok {
								parentsMap[attachmentMessageID] = types.Void
								parents = append(parents, attachmentMessageID)
							}
							added = true
						}
					})

					if added {
						break
					}
				}
			}
		} else {
			// if there are more than 8 referenced transactions:
			// for now we simply select as many parents as possible and hope all transactions will be covered
			count = t.tangle.Options.TipManagerParams.MaxParentsCount
		}
	}

	// nothing to do anymore
	if len(parents) == t.tangle.Options.TipManagerParams.MaxParentsCount {
		return
	}

	// select some current tips (depending on length of parents)
	if count+len(parents) > t.tangle.Options.TipManagerParams.MaxParentsCount {
		count = t.tangle.Options.TipManagerParams.MaxParentsCount - len(parents)
	}

	tips := t.tips.RandomUniqueEntries(count)
	// count is invalid or there are no tips
	if len(tips) == 0 {
		// only add genesis if no tip was found and not previously referenced (in case of a transaction)
		if len(parents) == 0 {
			parents = append(parents, EmptyMessageID)
		}
		return
	}
	// at least one tip is returned
	for _, tip := range tips {
		messageID := tip.(MessageID)

		if _, ok := parentsMap[messageID]; !ok {
			parentsMap[messageID] = types.Void
			parents = append(parents, messageID)
		}
	}

	return
}

// AllTips returns a list of all tips that are stored in the TipManger.
func (t *TipManager) AllTips() MessageIDs {
	return retrieveAllTips(t.tips)
}

func retrieveAllTips(tipsMap *randommap.RandomMap) MessageIDs {
	mapKeys := tipsMap.Keys()
	tips := make(MessageIDs, len(mapKeys))
	for i, key := range mapKeys {
		tips[i] = key.(MessageID)
	}
	return tips
}

// TipCount the amount of strong tips.
func (t *TipManager) TipCount() int {
	return t.tips.Size()
}

// Shutdown stops the TipManager.
func (t *TipManager) Shutdown() {
	t.tipsCleaner.Shutdown(timedexecutor.CancelPendingTasks)
}

// endregion ///////////////////////////////////////////////////////////////////////////////////////////////////////////

// region TipManagerEvents /////////////////////////////////////////////////////////////////////////////////////////////

// TipManagerEvents represents events happening on the TipManager.
type TipManagerEvents struct {
	// Fired when a tip is added.
	TipAdded *events.Event

	// Fired when a tip is removed.
	TipRemoved *events.Event
}

// endregion ///////////////////////////////////////////////////////////////////////////////////////////////////////////

// region TipEvent /////////////////////////////////////////////////////////////////////////////////////////////////////

// TipEvent holds the information provided by the TipEvent event that gets triggered when a message gets added or
// removed as tip.
type TipEvent struct {
	// MessageID of the added/removed tip.
	MessageID MessageID
}

func tipEventHandler(handler interface{}, params ...interface{}) {
	handler.(func(event *TipEvent))(params[0].(*TipEvent))
}

// endregion ///////////////////////////////////////////////////////////////////////////////////////////////////////////
