package tangle

import (
	"bytes"
	"crypto/rand"
	"fmt"
	"sort"
	"strings"
	"testing"
	"time"

	"github.com/cockroachdb/errors"
	"github.com/iotaledger/hive.go/cerrors"
	"github.com/iotaledger/hive.go/crypto/ed25519"
	"github.com/iotaledger/hive.go/identity"
	"github.com/iotaledger/hive.go/marshalutil"
	"github.com/mr-tron/base58"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/iotaledger/goshimmer/packages/ledgerstate"
	"github.com/iotaledger/goshimmer/packages/tangle/payload"
)

func randomBytes(size uint) []byte {
	buffer := make([]byte, size)
	_, err := rand.Read(buffer)
	if err != nil {
		panic(err)
	}
	return buffer
}

func randomMessageID() MessageID {
	msgBytes := randomBytes(MessageIDLength)
	result, _, _ := MessageIDFromBytes(msgBytes)
	return result
}

func randomParents(count int) MessageIDs {
	parents := make([]MessageID, 0, count)
	for i := 0; i < count; i++ {
		parents = append(parents, randomMessageID())
	}
	return parents
}

func testAreParentsSorted(parents []MessageID) bool {
	return sort.SliceIsSorted(parents, func(i, j int) bool {
		return bytes.Compare(parents[i].Bytes(), parents[j].Bytes()) < 0
	})
}

func testSortParents(parents []MessageID) []MessageID {
	sort.Slice(parents, func(i, j int) bool {
		return bytes.Compare(parents[i].Bytes(), parents[j].Bytes()) < 0
	})
	return parents
}

func TestNewMessageID(t *testing.T) {
	t.Run("CASE: Happy path", func(t *testing.T) {
		randID := randomMessageID()
		randIDString := randID.Base58()

		result, err := NewMessageID(randIDString)
		assert.NoError(t, err)
		assert.Equal(t, randID, result)
	})

	t.Run("CASE: Not base58 encoded", func(t *testing.T) {
		result, err := NewMessageID("O0l")
		assert.Error(t, err)
		assert.True(t, strings.Contains(err.Error(), "failed to decode base58 encoded string"))
		assert.Equal(t, EmptyMessageID, result)
	})

	t.Run("CASE: Too long string", func(t *testing.T) {
		result, err := NewMessageID(base58.Encode(randomBytes(MessageIDLength + 1)))
		assert.Error(t, err)
		assert.True(t, strings.Contains(err.Error(), "length of base58 formatted message id is wrong"))
		assert.Equal(t, EmptyMessageID, result)
	})
}

func TestMessageIDFromBytes(t *testing.T) {
	t.Run("CASE: Happy path", func(t *testing.T) {
		buffer := randomBytes(MessageIDLength)
		result, consumed, err := MessageIDFromBytes(buffer)
		assert.NoError(t, err)
		assert.Equal(t, MessageIDLength, consumed)
		assert.Equal(t, result.Bytes(), buffer)
	})

	t.Run("CASE: Too few bytes", func(t *testing.T) {
		buffer := randomBytes(MessageIDLength - 1)
		result, consumed, err := MessageIDFromBytes(buffer)
		assert.Error(t, err)
		assert.True(t, strings.Contains(err.Error(), "bytes not long enough"))
		assert.Equal(t, 0, consumed)
		assert.Equal(t, EmptyMessageID, result)
	})

	t.Run("CASE: More bytes", func(t *testing.T) {
		buffer := randomBytes(MessageIDLength + 1)
		result, consumed, err := MessageIDFromBytes(buffer)
		assert.NoError(t, err)
		assert.Equal(t, MessageIDLength, consumed)
		assert.Equal(t, buffer[:32], result.Bytes())
	})
}

func TestMessageIDFromMarshalUtil(t *testing.T) {
	t.Run("CASE: Happy path", func(t *testing.T) {
		randID := randomMessageID()
		marshalUtil := marshalutil.New(randID.Bytes())
		result, err := ReferenceFromMarshalUtil(marshalUtil)
		assert.NoError(t, err)
		assert.Equal(t, randID, result)
	})

	t.Run("CASE: Wrong bytes in MarshalUtil", func(t *testing.T) {
		marshalUtil := marshalutil.New(randomBytes(MessageIDLength - 1))
		result, err := ReferenceFromMarshalUtil(marshalUtil)
		assert.Error(t, err)
		assert.True(t, strings.Contains(err.Error(), "failed to parse message ID"))
		assert.Equal(t, EmptyMessageID, result)
	})
}

func TestMessageID_MarshalBinary(t *testing.T) {
	t.Run("CASE: Happy path", func(t *testing.T) {
		randID := randomMessageID()
		result, err := randID.MarshalBinary()
		assert.NoError(t, err)
		assert.Equal(t, randID.Bytes(), result)
	})
}

func TestMessageID_UnmarshalBinary(t *testing.T) {
	t.Run("CASE: Happy path", func(t *testing.T) {
		randID1 := randomMessageID()
		randID2 := randomMessageID()
		err := randID1.UnmarshalBinary(randID2.Bytes())
		assert.NoError(t, err)
		assert.Equal(t, randID1, randID2)
	})

	t.Run("CASE: Wrong length (less)", func(t *testing.T) {
		randID := randomMessageID()
		originalBytes := randID.Bytes()
		err := randID.UnmarshalBinary(randomBytes(MessageIDLength - 1))
		assert.Error(t, err)
		assert.True(t, strings.Contains(err.Error(), fmt.Sprintf("data must be exactly %d long to encode a valid message id", MessageIDLength)))
		assert.Equal(t, originalBytes, randID.Bytes())
	})

	t.Run("CASE: Wrong length (more)", func(t *testing.T) {
		randID := randomMessageID()
		originalBytes := randID.Bytes()
		err := randID.UnmarshalBinary(randomBytes(MessageIDLength + 1))
		assert.Error(t, err)
		assert.True(t, strings.Contains(err.Error(), fmt.Sprintf("data must be exactly %d long to encode a valid message id", MessageIDLength)))
		assert.Equal(t, originalBytes, randID.Bytes())
	})
}

func TestMessageID_String(t *testing.T) {
	randID := randomMessageID()
	randIDString := randID.String()
	assert.Equal(t, "MessageID("+base58.Encode(randID.Bytes())+")", randIDString)
}

func TestMessageID_Base58(t *testing.T) {
	randID := randomMessageID()
	randIDString := randID.Base58()
	assert.Equal(t, base58.Encode(randID.Bytes()), randIDString)
}

func TestMessage_VerifySignature(t *testing.T) {
	keyPair := ed25519.GenerateKeyPair()
	pl := payload.NewGenericDataPayload([]byte("test"))

	unsigned, _ := NewMessage(MessageIDs{EmptyMessageID}, MessageIDs{}, MessageIDs{}, MessageIDs{}, time.Time{}, keyPair.PublicKey, 0, pl, 0, ed25519.Signature{})
	assert.False(t, unsigned.VerifySignature())

	unsignedBytes := unsigned.Bytes()
	signature := keyPair.PrivateKey.Sign(unsignedBytes[:len(unsignedBytes)-ed25519.SignatureSize])

	signed, _ := NewMessage(MessageIDs{EmptyMessageID}, MessageIDs{}, MessageIDs{}, MessageIDs{}, time.Time{}, keyPair.PublicKey, 0, pl, 0, signature)
	assert.True(t, signed.VerifySignature())
}

func TestMessage_UnmarshalTransaction(t *testing.T) {
	tangle := NewTestTangle()
	defer tangle.Shutdown()

	testMessage, err := NewMessage(randomParents(1),
		randomParents(1),
		MessageIDs{},
		MessageIDs{},
		time.Now(),
		ed25519.PublicKey{},
		0,
		randomTransaction(),
		0,
		ed25519.Signature{})
	assert.NoError(t, err)

	restoredMessage, _, err := MessageFromBytes(testMessage.Bytes())
	assert.NoError(t, err)
	assert.Equal(t, testMessage.ID(), restoredMessage.ID())
}

func TestMessage_MarshalUnmarshal(t *testing.T) {
	tangle := NewTestTangle()
	defer tangle.Shutdown()

	tangle.MessageFactory.likeReferencesFunc = emptyLikeReferences

	testMessage, err := tangle.MessageFactory.IssuePayload(payload.NewGenericDataPayload([]byte("test")))
	require.NoError(t, err)
	assert.Equal(t, true, testMessage.VerifySignature())

	t.Log(testMessage)

	restoredMessage, _, err := MessageFromBytes(testMessage.Bytes())
	if assert.NoError(t, err, err) {
		assert.Equal(t, testMessage.ID(), restoredMessage.ID())
		assert.ElementsMatch(t, testMessage.ParentsByType(StrongParentType), restoredMessage.ParentsByType(StrongParentType))
		assert.ElementsMatch(t, testMessage.ParentsByType(WeakParentType), restoredMessage.ParentsByType(WeakParentType))
		assert.Equal(t, testMessage.IssuerPublicKey(), restoredMessage.IssuerPublicKey())
		assert.Equal(t, testMessage.IssuingTime().Round(time.Second), restoredMessage.IssuingTime().Round(time.Second))
		assert.Equal(t, testMessage.SequenceNumber(), restoredMessage.SequenceNumber())
		assert.Equal(t, testMessage.Nonce(), restoredMessage.Nonce())
		assert.Equal(t, testMessage.Signature(), restoredMessage.Signature())
		assert.Equal(t, true, restoredMessage.VerifySignature())
	}
}

func TestNewMessageWithValidation(t *testing.T) {
	t.Run("CASE: Too many strong parents", func(t *testing.T) {
		// too many strong parents
		strongParents := testSortParents(randomParents(MaxParentsCount + 1))
		block := ParentsBlock{
			ParentsType: StrongParentType,
			References:  strongParents,
		}

		_, err := newMessageWithValidation(
			MessageVersion,
			[]ParentsBlock{block},
			time.Now(),
			ed25519.PublicKey{},
			payload.NewGenericDataPayload([]byte("")),
			0,
			ed25519.Signature{},
			0,
		)
		assert.ErrorIs(t, err, ErrParentsOutOfRange)
	})

	t.Run("CASE: Nil block", func(t *testing.T) {
		_, err := newMessageWithValidation(
			MessageVersion,
			nil,
			time.Now(),
			ed25519.PublicKey{},
			payload.NewGenericDataPayload([]byte("")),
			0,
			ed25519.Signature{},
			0,
		)
		assert.ErrorIs(t, err, ErrNoStrongParents)
	})

	t.Run("CASE: Empty Block", func(t *testing.T) {
		block := ParentsBlock{}

		_, err := newMessageWithValidation(
			MessageVersion,
			[]ParentsBlock{block},
			time.Now(),
			ed25519.PublicKey{},
			payload.NewGenericDataPayload([]byte("")),
			0,
			ed25519.Signature{},
			0,
		)
		assert.ErrorIs(t, err, ErrNoStrongParents)
	})

	t.Run("CASE: Blocks are unordered", func(t *testing.T) {
		parents := testSortParents(randomParents(MaxParentsCount))

		strongBlock := ParentsBlock{
			ParentsType: StrongParentType,
			References:  parents,
		}
		weakBlock := ParentsBlock{
			ParentsType: WeakParentType,
			References:  parents,
		}
		dislikeBlock := ParentsBlock{
			ParentsType: DislikeParentType,
			References:  parents,
		}
		likeBlock := ParentsBlock{
			ParentsType: LikeParentType,
			References:  parents,
		}

		_, err := newMessageWithValidation(
			MessageVersion,
			[]ParentsBlock{weakBlock, strongBlock, dislikeBlock, likeBlock},
			time.Now(),
			ed25519.PublicKey{},
			payload.NewGenericDataPayload([]byte("")),
			0,
			ed25519.Signature{},
			0,
		)
		// Since no strong parents in first block the validator will assume they are missing
		assert.ErrorIs(t, err, ErrNoStrongParents, "weak block came before strong block")

		_, err = newMessageWithValidation(
			MessageVersion,
			[]ParentsBlock{strongBlock, dislikeBlock, weakBlock, likeBlock},
			time.Now(),
			ed25519.PublicKey{},
			payload.NewGenericDataPayload([]byte("")),
			0,
			ed25519.Signature{},
			0,
		)
		assert.ErrorIs(t, err, ErrBlocksNotOrderedByType, "dislike block came before weak block")

		_, err = newMessageWithValidation(
			MessageVersion,
			[]ParentsBlock{strongBlock, dislikeBlock, weakBlock, likeBlock},
			time.Now(),
			ed25519.PublicKey{},
			payload.NewGenericDataPayload([]byte("")),
			0,
			ed25519.Signature{},
			0,
		)
		assert.ErrorIs(t, err, ErrBlocksNotOrderedByType, "dislike block came before weak block")

		_, err = newMessageWithValidation(
			MessageVersion,
			[]ParentsBlock{strongBlock, weakBlock, likeBlock, dislikeBlock},
			time.Now(),
			ed25519.PublicKey{},
			payload.NewGenericDataPayload([]byte("")),
			0,
			ed25519.Signature{},
			0,
		)
		assert.ErrorIs(t, err, ErrBlocksNotOrderedByType, "like block came before dislike block")
	})

	t.Run("CASE: Repeating block types", func(t *testing.T) {
		parents := testSortParents(randomParents(MaxParentsCount))

		strongBlock := ParentsBlock{
			ParentsType: StrongParentType,
			References:  parents,
		}
		strongBlock2 := ParentsBlock{
			ParentsType: StrongParentType,
			References:  parents,
		}
		likeBlock := ParentsBlock{
			ParentsType: LikeParentType,
			References:  parents,
		}
		likeBlock2 := ParentsBlock{
			ParentsType: LikeParentType,
			References:  parents,
		}

		_, err := newMessageWithValidation(
			MessageVersion,
			[]ParentsBlock{strongBlock, strongBlock2, likeBlock},
			time.Now(),
			ed25519.PublicKey{},
			payload.NewGenericDataPayload([]byte("")),
			0,
			ed25519.Signature{},
			0,
		)

		assert.ErrorIs(t, err, ErrRepeatingBlockTypes, "strong block repeats")

		_, err = newMessageWithValidation(
			MessageVersion,
			[]ParentsBlock{strongBlock, likeBlock, likeBlock2},
			time.Now(),
			ed25519.PublicKey{},
			payload.NewGenericDataPayload([]byte("")),
			0,
			ed25519.Signature{},
			0,
		)

		assert.ErrorIs(t, err, ErrRepeatingBlockTypes, "like block repeats")
	})

	t.Run("CASE: Unknown block type", func(t *testing.T) {
		parents := testSortParents(randomParents(MaxParentsCount))

		strongBlock := ParentsBlock{
			ParentsType: StrongParentType,
			References:  parents,
		}
		likeBlock := ParentsBlock{
			ParentsType: LikeParentType,
			References:  parents,
		}
		unknownBlock := ParentsBlock{
			ParentsType: NumberOfBlockTypes, // this should always be out of range
			References:  parents,
		}

		_, err := newMessageWithValidation(
			MessageVersion,
			[]ParentsBlock{strongBlock, likeBlock, unknownBlock},
			time.Now(),
			ed25519.PublicKey{},
			payload.NewGenericDataPayload([]byte("")),
			0,
			ed25519.Signature{},
			0,
		)

		assert.ErrorIs(t, err, ErrBlockTypeIsUnknown)
	})

	t.Run("Case: Duplicate references", func(t *testing.T) {
		parents := testSortParents(randomParents(4))
		parents = append(parents, parents[3])

		strongBlock := ParentsBlock{
			ParentsType: StrongParentType,
			References:  parents,
		}

		_, err := newMessageWithValidation(
			MessageVersion,
			[]ParentsBlock{strongBlock},
			time.Now(),
			ed25519.PublicKey{},
			payload.NewGenericDataPayload([]byte("")),
			0,
			ed25519.Signature{},
			0,
		)
		assert.ErrorIs(t, err, ErrRepeatingReferencesInBlock)

		parents = testSortParents(randomParents(4))
		parents = append(parents, parents[1])

		strongBlock.References = parents

		_, err = newMessageWithValidation(
			MessageVersion,
			[]ParentsBlock{strongBlock},
			time.Now(),
			ed25519.PublicKey{},
			payload.NewGenericDataPayload([]byte("")),
			0,
			ed25519.Signature{},
			0,
		)
		// if the duplicates are not consecutive a lexicographically order error is returned
		assert.ErrorIs(t, err, ErrParentsNotLexicographicallyOrdered)
	})

	t.Run("Parents Repeating across blocks", func(t *testing.T) {
		parents := testSortParents(randomParents(4))
		strongBlock := ParentsBlock{
			ParentsType: StrongParentType,
			References:  parents,
		}

		likeBlock := ParentsBlock{
			ParentsType: LikeParentType,
			References:  parents,
		}

		_, err := newMessageWithValidation(
			MessageVersion,
			[]ParentsBlock{strongBlock, likeBlock},
			time.Now(),
			ed25519.PublicKey{},
			payload.NewGenericDataPayload([]byte("")),
			0,
			ed25519.Signature{},
			0,
		)

		assert.NoError(t, err, "strong and like parents may have duplicate parents")

		weakBlock := ParentsBlock{
			ParentsType: WeakParentType,
			References:  parents,
		}

		_, err = newMessageWithValidation(
			MessageVersion,
			[]ParentsBlock{strongBlock, weakBlock},
			time.Now(),
			ed25519.PublicKey{},
			payload.NewGenericDataPayload([]byte("")),
			0,
			ed25519.Signature{},
			0,
		)

		assert.ErrorIs(t, err, ErrRepeatingMessagesAcrossBlocks, "messages repeating in weak and strong block")

		// check for repeating message across weak and dislike block
		weakParents := testSortParents(randomParents(4))
		dislikeParents := randomParents(4)
		// create duplicate
		dislikeParents[2] = weakParents[2]
		dislikeParents = testSortParents(dislikeParents)

		weakBlock = ParentsBlock{
			ParentsType: WeakParentType,
			References:  weakParents,
		}

		dislikeBlock := ParentsBlock{
			ParentsType: LikeParentType,
			References:  dislikeParents,
		}

		_, err = newMessageWithValidation(
			MessageVersion,
			[]ParentsBlock{strongBlock, weakBlock, dislikeBlock},
			time.Now(),
			ed25519.PublicKey{},
			payload.NewGenericDataPayload([]byte("")),
			0,
			ed25519.Signature{},
			0)

		assert.ErrorIs(t, err, ErrRepeatingMessagesAcrossBlocks, "message repeated across weak and dislike blocks")
	})
}

func TestMessage_NewMessage(t *testing.T) {
	t.Run("CASE: No parents at all", func(t *testing.T) {
		_, err := NewMessage(
			nil,
			nil,
			nil,
			nil,
			time.Now(),
			ed25519.PublicKey{},
			0,
			payload.NewGenericDataPayload([]byte("")),
			0,
			ed25519.Signature{},
		)
		assert.ErrorIs(t, err, ErrNoStrongParents)
	})

	t.Run("CASE: Minimum number of parents", func(t *testing.T) {
		_, err := NewMessage(
			[]MessageID{EmptyMessageID},
			nil,
			nil,
			nil,
			time.Now(),
			ed25519.PublicKey{},
			0,
			payload.NewGenericDataPayload([]byte("")),
			0,
			ed25519.Signature{},
		)
		// should pass since EmptyMessageId is a valid MessageId
		assert.NoError(t, err)
	})

	t.Run("CASE: Maximum number of parents (only strong)", func(t *testing.T) {
		// max number of parents supplied (only strong)
		strongParents := randomParents(MaxParentsCount)
		_, err := NewMessage(
			strongParents,
			nil,
			nil,
			nil,
			time.Now(),
			ed25519.PublicKey{},
			0,
			payload.NewGenericDataPayload([]byte("")),
			0,
			ed25519.Signature{},
		)
		assert.NoError(t, err)
	})

	t.Run("CASE: Maximum number of weak parents (one strong)", func(t *testing.T) {
		// max number of weak parents plus one strong
		weakParents := randomParents(MaxParentsCount)
		_, err := NewMessage(
			[]MessageID{EmptyMessageID},
			weakParents,
			nil,
			nil,
			time.Now(),
			ed25519.PublicKey{},
			0,
			payload.NewGenericDataPayload([]byte("")),
			0,
			ed25519.Signature{},
		)
		assert.NoError(t, err)
	})

	t.Run("CASE: Too many parents, but okay without duplicates", func(t *testing.T) {
		strongParents := randomParents(MaxParentsCount)
		// MaxParentsCount + 1 parents, but there is one duplicate
		strongParents = append(strongParents, strongParents[MaxParentsCount-1])
		_, err := NewMessage(
			strongParents,
			nil,
			nil,
			nil,
			time.Now(),
			ed25519.PublicKey{},
			0,
			payload.NewGenericDataPayload([]byte("")),
			0,
			ed25519.Signature{},
		)
		assert.NoError(t, err)
	})

	t.Run("CASE: Duplicate strong parents", func(t *testing.T) {
		// max number of parents supplied (only strong)
		strongParents := randomParents(MaxParentsCount / 2)

		strongParents = append(strongParents, strongParents...)

		msg, err := NewMessage(
			strongParents,
			nil,
			nil,
			nil,
			time.Now(),
			ed25519.PublicKey{},
			0,
			payload.NewGenericDataPayload([]byte("")),
			0,
			ed25519.Signature{},
		)
		assert.NoError(t, err)

		msgStrongParents := msg.ParentsByType(StrongParentType)
		assert.True(t, testAreParentsSorted(msgStrongParents))
		assert.Equal(t, MaxParentsCount/2, len(msgStrongParents))
	})

	t.Run("CASE: Duplicate weak parents", func(t *testing.T) {
		weakParents := randomParents(3)
		weakParents = append(weakParents, weakParents...)
		msg, err := NewMessage(
			[]MessageID{EmptyMessageID},
			weakParents,
			nil,
			nil,
			time.Now(),
			ed25519.PublicKey{},
			0,
			payload.NewGenericDataPayload([]byte("")),
			0,
			ed25519.Signature{},
		)
		assert.NoError(t, err, "Syntactically invalid message")

		msgWeakParents := msg.ParentsByType(WeakParentType)
		assert.True(t, testAreParentsSorted(msgWeakParents))
		assert.Equal(t, 3, len(msgWeakParents))
	})
}

func TestMessage_Bytes(t *testing.T) {
	t.Run("CASE: Parents not sorted", func(t *testing.T) {
		msg, err := NewMessage(
			randomParents(4),
			randomParents(4),
			nil,
			nil,
			time.Now(),
			ed25519.PublicKey{},
			0,
			payload.NewGenericDataPayload([]byte("")),
			0,
			ed25519.Signature{},
		)
		assert.NoError(t, err)

		msgBytes := msg.Bytes()
		// bytes 4 to 260 hold the 8 parent IDs
		// manually change their order
		tmp := make([]byte, 32)
		copy(tmp, msgBytes[3:35])
		copy(msgBytes[3:35], msgBytes[3+32:35+32])
		copy(msgBytes[3+32:35+32], tmp)
		_, _, err = MessageFromBytes(msgBytes)
		assert.Error(t, err)
	})

	t.Run("CASE: Max msg size", func(t *testing.T) {
		// 4 bytes for payload size field
		data := make([]byte, payload.MaxSize-4)
		msg, err := NewMessage(
			randomParents(MaxParentsCount),
			randomParents(MaxParentsCount),
			randomParents(MaxParentsCount),
			randomParents(MaxParentsCount),
			time.Now(),
			ed25519.PublicKey{},
			0,
			payload.NewGenericDataPayload(data),
			0,
			ed25519.Signature{},
		)
		assert.NoError(t, err)

		msgBytes := msg.Bytes()
		assert.Equal(t, MaxMessageSize, len(msgBytes))
	})

	t.Run("CASE: Min msg size", func(t *testing.T) {
		// msg with minimum number of parents
		msg, err := NewMessage(
			randomParents(MinParentsCount),
			nil,
			nil,
			nil,
			time.Now(),
			ed25519.PublicKey{},
			0,
			payload.NewGenericDataPayload(nil),
			0,
			ed25519.Signature{},
		)
		assert.NoError(t, err)

		t.Logf("%s", msg)
		msgBytes := msg.Bytes()
		// 4 full parents blocks - 1 parent block with 1 parent
		assert.Equal(t, MaxMessageSize-payload.MaxSize+4-(3*(1+1+8*32)+(7*32)), len(msgBytes))
	})
}

func TestMessageFromBytes(t *testing.T) {
	t.Run("CASE: Happy path", func(t *testing.T) {
		msg, err := NewMessage(
			randomParents(MaxParentsCount/2),
			randomParents(MaxParentsCount/2),
			nil,
			nil,
			time.Now(),
			ed25519.PublicKey{},
			0,
			payload.NewGenericDataPayload([]byte("This is a test message.")),
			0,
			ed25519.Signature{},
		)
		assert.NoError(t, err)

		msgBytes := msg.Bytes()
		result, consumedBytes, err := MessageFromBytes(msgBytes)
		assert.Equal(t, len(msgBytes), consumedBytes)
		assert.NoError(t, err)
		assert.Equal(t, msg.Version(), result.Version())
		assert.Equal(t, msg.ParentsByType(StrongParentType), result.ParentsByType(StrongParentType))
		assert.Equal(t, msg.ParentsByType(WeakParentType), result.ParentsByType(WeakParentType))
		// TODO
		// assert.Equal(t, msg.ParentsCount(), result.ParentsCount())
		assert.Equal(t, msg.IssuerPublicKey(), result.IssuerPublicKey())
		// time is in different representation but it denotes the same time
		assert.True(t, msg.IssuingTime().Equal(result.IssuingTime()))
		assert.Equal(t, msg.SequenceNumber(), result.SequenceNumber())
		assert.Equal(t, msg.Payload(), result.Payload())
		assert.Equal(t, msg.Nonce(), result.Nonce())
		assert.Equal(t, msg.Signature(), result.Signature())
		assert.Equal(t, msg.calculateID(), result.calculateID())
	})

	t.Run("CASE: Trailing bytes", func(t *testing.T) {
		msg, err := NewMessage(
			randomParents(MaxParentsCount/2),
			randomParents(MaxParentsCount/2),
			nil,
			nil,
			time.Now(),
			ed25519.PublicKey{},
			0,
			payload.NewGenericDataPayload([]byte("This is a test message.")),
			0,
			ed25519.Signature{},
		)
		assert.NoError(t, err, "Syntactically invalid message created")
		msgBytes := msg.Bytes()
		// put some bytes at the end
		msgBytes = append(msgBytes, []byte{0, 1, 2, 3, 4}...)
		_, _, err = MessageFromBytes(msgBytes)
		assert.Error(t, err)
		assert.True(t, errors.Is(err, cerrors.ErrParseBytesFailed))
	})
}

func createTestMsgBytes(numStrongParents int, numWeakParents int) []byte {
	msg, _ := NewMessage(
		randomParents(numStrongParents),
		randomParents(numWeakParents),
		nil,
		nil,
		time.Now(),
		ed25519.PublicKey{},
		0,
		payload.NewGenericDataPayload([]byte("This is a test message.")),
		0,
		ed25519.Signature{},
	)

	return msg.Bytes()
}

func TestMessageFromMarshalUtil(t *testing.T) {
	t.Run("CASE: Missing version", func(t *testing.T) {
		marshaller := marshalutil.New([]byte{})
		// missing version
		_, err := MessageFromMarshalUtil(marshaller)
		assert.Error(t, err)
		assert.True(t, strings.Contains(err.Error(), "failed to parse message version"))
	})

	t.Run("CASE: Missing parents count", func(t *testing.T) {
		msgBytes := createTestMsgBytes(MaxParentsCount/2, MaxParentsCount/2)
		// missing parentsCount
		marshaller := marshalutil.New(msgBytes[:1])
		_, err := MessageFromMarshalUtil(marshaller)
		assert.Error(t, err)
		assert.True(t, strings.Contains(err.Error(), "failed to parse parents count"))
	})

	t.Run("CASE: Invalid parents count (less)", func(t *testing.T) {
		msgBytes := createTestMsgBytes(MaxParentsCount/2, MaxParentsCount/2)
		msgBytes[1] = byte(MinParentsCount) - 1
		marshaller := marshalutil.New(msgBytes[:2])
		_, err := MessageFromMarshalUtil(marshaller)
		assert.Error(t, err)
		assert.EqualError(t, err, fmt.Sprintf("parents count %d not allowed: failed to parse bytes", MinParentsCount-1))
	})

	t.Run("CASE: Invalid parents count (more)", func(t *testing.T) {
		msgBytes := createTestMsgBytes(MaxParentsCount/2, MaxParentsCount/2)
		msgBytes[1] = byte(MaxParentsCount) + 1
		marshaller := marshalutil.New(msgBytes[:2])
		_, err := MessageFromMarshalUtil(marshaller)
		assert.Error(t, err)
		assert.EqualError(t, err, fmt.Sprintf("parents count %d not allowed: failed to parse bytes", MaxParentsCount+1))
	})

	t.Run("CASE: Missing parent types", func(t *testing.T) {
		msgBytes := createTestMsgBytes(MaxParentsCount/2, MaxParentsCount/2)
		marshaller := marshalutil.New(msgBytes[:2])
		_, err := MessageFromMarshalUtil(marshaller)
		assert.Error(t, err)
		assert.True(t, strings.Contains(err.Error(), "failed to parse parent types"))
	})

	t.Run("CASE: Missing parents (all)", func(t *testing.T) {
		msgBytes := createTestMsgBytes(MaxParentsCount/2, MaxParentsCount/2)
		marshaller := marshalutil.New(msgBytes[:3])
		_, err := MessageFromMarshalUtil(marshaller)
		assert.Error(t, err)
		assert.True(t, strings.Contains(err.Error(), "failed to parse parent"))
	})

	t.Run("CASE: Missing parents (one)", func(t *testing.T) {
		msgBytes := createTestMsgBytes(MaxParentsCount/2, MaxParentsCount/2)
		marshaller := marshalutil.New(msgBytes[:3+(MaxParentsCount-1)*32])
		_, err := MessageFromMarshalUtil(marshaller)
		assert.Error(t, err)
		assert.True(t, strings.Contains(err.Error(), "failed to parse parent"))
	})
}

func TestMessage_ForEachStrongParent(t *testing.T) {
	t.Run("Happy path", func(t *testing.T) {
		strongParents := randomParents(MaxParentsCount / 2)
		weakParents := randomParents(MaxParentsCount / 2)

		msg, err := NewMessage(
			strongParents,
			weakParents,
			nil,
			nil,
			time.Now(),
			ed25519.PublicKey{},
			666,
			payload.NewGenericDataPayload([]byte("This is a test message.")),
			99,
			ed25519.Signature{},
		)
		assert.NoError(t, err)

		sortedStrongParents := sortParents(strongParents)
		resultParents := make(MessageIDs, 0)
		checker := func(parent MessageID) {
			resultParents = append(resultParents, parent)
		}
		msg.ForEachParentByType(StrongParentType, checker)

		assert.Equal(t, sortedStrongParents, resultParents)
	})
}

func TestMessage_ForEachWeakParent(t *testing.T) {
	t.Run("Happy path", func(t *testing.T) {
		strongParents := randomParents(MaxParentsCount / 2)
		weakParents := randomParents(MaxParentsCount / 2)

		msg, err := NewMessage(
			strongParents,
			weakParents,
			nil,
			nil,
			time.Now(),
			ed25519.PublicKey{},
			666,
			payload.NewGenericDataPayload([]byte("This is a test message.")),
			99,
			ed25519.Signature{},
		)
		assert.NoError(t, err)

		sortedWeakParents := sortParents(weakParents)
		resultParents := make(MessageIDs, 0)
		checker := func(parent MessageID) {
			resultParents = append(resultParents, parent)
		}
		msg.ForEachParentByType(WeakParentType, checker)

		assert.Equal(t, sortedWeakParents, resultParents)
	})
}

func randomTransaction() *ledgerstate.Transaction {
	ID, _ := identity.RandomID()
	input := ledgerstate.NewUTXOInput(ledgerstate.EmptyOutputID)
	var outputs ledgerstate.Outputs
	seed := ed25519.NewSeed()
	w := wl{
		keyPair: *seed.KeyPair(0),
		address: ledgerstate.NewED25519Address(seed.KeyPair(0).PublicKey),
	}
	output := ledgerstate.NewSigLockedColoredOutput(ledgerstate.NewColoredBalances(map[ledgerstate.Color]uint64{
		ledgerstate.ColorIOTA: uint64(100),
	}), w.address)
	outputs = append(outputs, output)
	essence := ledgerstate.NewTransactionEssence(1, time.Now(), ID, ID, ledgerstate.NewInputs(input), outputs)

	unlockBlock := ledgerstate.NewSignatureUnlockBlock(w.sign(essence))

	return ledgerstate.NewTransaction(essence, ledgerstate.UnlockBlocks{unlockBlock})
}

type wl struct {
	keyPair ed25519.KeyPair
	address *ledgerstate.ED25519Address
}

func (w wl) privateKey() ed25519.PrivateKey {
	return w.keyPair.PrivateKey
}

func (w wl) publicKey() ed25519.PublicKey {
	return w.keyPair.PublicKey
}

func (w wl) sign(txEssence *ledgerstate.TransactionEssence) *ledgerstate.ED25519Signature {
	return ledgerstate.NewED25519Signature(w.publicKey(), w.privateKey().Sign(txEssence.Bytes()))
}
