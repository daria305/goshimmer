package tangle

import (
	"fmt"
	"github.com/iotaledger/goshimmer/packages/tangle/payload"
)

type TipManagerOrphanageAttack struct {
	*TipManager
}

// NewTipManagerOrphanageAttack creates a new tip-selector.
func NewTipManagerOrphanageAttack(legitManager *TipManager) *TipManagerOrphanageAttack {
	maliciousManager := &TipManagerOrphanageAttack{
		legitManager,
	}
	return maliciousManager
}

func (t *TipManagerOrphanageAttack) Setup() {
	t.TipManager.Setup()
	fmt.Println("Malicious printing in the Tip Manager Setup()!")
}

func (t *TipManagerOrphanageAttack) Tips(p payload.Payload, countParents int) (parents MessageIDs, err error) {
	fmt.Println("Malicious printing Tips() hacked!")
	return t.TipManager.Tips(p, countParents)
}
