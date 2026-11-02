package bridge

import (
	"strconv"
	"strings"
	"testing"
	"time"
)

// The message log and the ledger are handed to their writers under the hub's lock: what that costs the hub (and so
// every report and page request waiting for the lock) with a full log, 12 tasks' worth of messages, and a ledger of
// 90 days' rows.
func BenchmarkSaveMessagesLocked(b *testing.B) {
	h := NewHub()
	h.dataDir = b.TempDir()
	for i := range 3900 {
		h.messages = append(h.messages, &Message{ID: newID(), From: "s" + strconv.Itoa(i%40), FromName: "alice-T1",
			To: "s" + strconv.Itoa((i+1)%40), ToName: "bob-T1", Text: strings.Repeat("a message between two sessions ", 8),
			Task: "T" + strconv.Itoa(i%12), Delivered: true, Received: true, Time: time.Now()})
	}
	b.ResetTimer()
	for range b.N {
		h.mu.Lock()
		h.saveMessagesLocked()
		h.mu.Unlock()
	}
	b.StopTimer()
	if err := h.msgSave.wait(); err != nil {
		b.Fatal(err)
	}
}

func BenchmarkSaveLedgerLocked(b *testing.B) {
	h := NewHub()
	h.dataDir = b.TempDir()
	for d := range 90 {
		for a := range 30 {
			h.ledger = append(h.ledger, &LedgerRow{Day: time.Now().AddDate(0, 0, -d).Format("2006-01-02"), Account: "acc" + strconv.Itoa(a),
				USD: 1.25, Turns: 40, Tools: 300})
		}
	}
	b.ResetTimer()
	for range b.N {
		h.mu.Lock()
		h.saveLedgerLocked()
		h.mu.Unlock()
	}
	b.StopTimer()
	if err := h.ledgerSave.wait(); err != nil {
		b.Fatal(err)
	}
}
