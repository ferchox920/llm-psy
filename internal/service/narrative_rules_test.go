package service

import (
	"strings"
	"testing"
)

func TestDeriveBondDynamicsModes(t *testing.T) {
	tests := []struct {
		name     string
		trust    int
		intimacy int
		respect  int
		mustHave []string
		mustNot  []string
	}{
		{
			name:     "toxic jealous mode",
			trust:    10,
			intimacy: 90,
			respect:  50,
			mustHave: []string{"MODO: CELOS"},
		},
		{
			name:     "no jealous mode when trust ok",
			trust:    50,
			intimacy: 90,
			respect:  50,
			mustNot:  []string{"MODO: CELOS"},
		},
		{
			name:     "jealous plus hostility when respect low",
			trust:    10,
			intimacy: 90,
			respect:  30,
			mustHave: []string{"CELOS", "HOSTILIDAD"},
		},
		{
			name:     "jealous mode at threshold",
			trust:    40,
			intimacy: 70,
			respect:  80,
			mustHave: []string{"MODO: CELOS"},
		},
		{
			name:     "hostility mode at new threshold",
			trust:    50,
			intimacy: 90,
			respect:  34,
			mustHave: []string{"HOSTILIDAD"},
		},
		{
			name:     "stable neutral when no modes apply",
			trust:    60,
			intimacy: 60,
			respect:  60,
			mustHave: []string{"estable/neutral"},
			mustNot:  []string{"MODO"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := deriveBondDynamics(tt.trust, tt.intimacy, tt.respect)
			for _, want := range tt.mustHave {
				if !strings.Contains(got, want) {
					t.Fatalf("deriveBondDynamics(%d,%d,%d) = %q; want contains %q", tt.trust, tt.intimacy, tt.respect, got, want)
				}
			}
			for _, forbidden := range tt.mustNot {
				if strings.Contains(got, forbidden) {
					t.Fatalf("deriveBondDynamics(%d,%d,%d) = %q; must not contain %q", tt.trust, tt.intimacy, tt.respect, got, forbidden)
				}
			}
		})
	}
}
