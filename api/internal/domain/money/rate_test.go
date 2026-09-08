package money_test

import (
	"errors"
	"testing"

	"github.com/ikmetrik/sms-platform/api/internal/domain/money"
)

func TestRateFromString(t *testing.T) {
	tests := []struct{ in, want string }{
		{"43.204512", "43.20451200"},
		{"43,204512", "43.20451200"}, // virgüllü ondalık kabul edilir
		{" 1.5 ", "1.50000000"},
		{"0", "0.00000000"},
	}
	for _, tt := range tests {
		r, err := money.RateFromString(tt.in)
		if err != nil {
			t.Fatalf("RateFromString(%q): %v", tt.in, err)
		}
		if got := r.String(); got != tt.want {
			t.Errorf("RateFromString(%q) = %s, beklenen %s", tt.in, got, tt.want)
		}
	}
	for _, bad := range []string{"", "abc", "1.2.3"} {
		if _, err := money.RateFromString(bad); err == nil {
			t.Errorf("RateFromString(%q) hata vermeliydi", bad)
		}
	}
}

func TestMarginRate(t *testing.T) {
	tests := []struct{ percent, want string }{
		{"40", "1.40000000"},
		{"0", "1.00000000"},
		{"12.5", "1.12500000"},
		{"100", "2.00000000"},
	}
	for _, tt := range tests {
		r, err := money.MarginRate(tt.percent)
		if err != nil {
			t.Fatalf("MarginRate(%q): %v", tt.percent, err)
		}
		if got := r.String(); got != tt.want {
			t.Errorf("MarginRate(%q) = %s, beklenen %s", tt.percent, got, tt.want)
		}
	}
	if _, err := money.MarginRate("-1"); !errors.Is(err, money.ErrNegativeRate) {
		t.Fatalf("negatif marj reddedilmeliydi: %v", err)
	}
}

func TestNewRate(t *testing.T) {
	r, err := money.NewRate(3, 4)
	if err != nil || r.String() != "0.75000000" {
		t.Fatalf("NewRate(3,4) = %v, %v", r.String(), err)
	}
	if _, err := money.NewRate(1, 0); err == nil {
		t.Fatal("sıfır payda hata vermeliydi")
	}
}

func TestRateHelpers(t *testing.T) {
	if !money.RateOne().Mul(money.RateOne()).IsZero() == false {
		t.Fatal("1 × 1 sıfır olmamalı")
	}
	if money.RateOne().IsNegative() {
		t.Fatal("1 negatif değil")
	}
	var zero money.Rate // sıfır değeri güvenli olmalı, panic etmemeli
	if !zero.IsZero() {
		t.Fatal("Rate sıfır değeri IsZero olmalı")
	}
	if zero.String() != "0.00000000" {
		t.Fatalf("sıfır Rate String() = %s", zero.String())
	}
	// 1.5 × 2 = 3
	a, _ := money.RateFromString("1.5")
	b, _ := money.RateFromString("2")
	if got := a.Mul(b).String(); got != "3.00000000" {
		t.Fatalf("1.5 × 2 = %s", got)
	}
}
