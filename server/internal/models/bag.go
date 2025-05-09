package models

import (
	"strings"
	"time"

	"github.com/the-clothing-loop/website/server/sharedtypes"
)

type Bag sharedtypes.Bag

func (b *Bag) AddLastUserEmailToUpdateFifo(email string) {
	var shouldAppendDate bool
	b.LastUserEmailToUpdate, shouldAppendDate = BagStringMaxAppend(b.LastUserEmailToUpdate, email)

	if shouldAppendDate {
		date := time.Now().Format(time.RFC3339)
		b.LastUserDateToUpdate, _ = BagStringMaxAppend(b.LastUserDateToUpdate, date)
	}
}

func BagStringMaxAppend(sList string, str string) (string, bool) {
	list := []string{}
	if sList != "" {
		list = strings.Split(sList, ",")
	}
	if len(list) > 0 {
		if str == list[len(list)-1] {
			// Last str is already there as the last value in the list
			return sList, false
		}
	}

	// only keeps 4 emails in total, this remove the oldest
	if len(list) >= 4 {
		list = list[len(list)-3:]
	}
	list = append(list, str)
	return strings.Join(list, ","), true
}
