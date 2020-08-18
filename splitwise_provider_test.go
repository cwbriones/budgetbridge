package main

import (
	"testing"
)

func TestNetBalanceToMilliunits(t *testing.T) {
	testcases := []struct {
		balance string
		units   int
		err     bool
	}{
		{
			balance: "81.1",
			units:   81100,
		},
		{
			balance: "1.23",
			units:   1230,
		},
		{
			balance: "-81.1",
			units:   -81100,
		},
		{
			balance: "-1.23",
			units:   -1230,
		},
	}
	for _, tc := range testcases {
		units, err := netBalanceToMilliUnits(tc.balance)
		if tc.err && err == nil {
			t.Errorf("Convert '%s': Expected error, got units %d", tc.balance, units)
		}
		if !tc.err && err == nil {
			if tc.units != units {
				t.Errorf("Convert %s: Expected units %d, got %d", tc.balance, tc.units, units)
			}
		}
	}
}
