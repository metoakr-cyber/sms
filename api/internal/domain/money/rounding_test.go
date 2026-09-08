package money_test

import (
	"errors"
	"math"
	"testing"

	"github.com/ikmetrik/sms-platform/api/internal/domain/money"
)

func TestRoundingDirections(t *testing.T) {
	// 100 kuruş × 1.005 = 100.5 kuruş — yuvarlama yönü sonucu belirler.
	base := money.New(100, money.TRY)
	rate, _ := money.RateFromString("1.005")

	up, err := base.MulRate(rate, money.RoundUp)
	if err != nil || up.Minor() != 101 {
		t.Fatalf("RoundUp = %d, beklenen 101 (%v)", up.Minor(), err)
	}
	down, err := base.MulRate(rate, money.RoundDown)
	if err != nil || down.Minor() != 100 {
		t.Fatalf("RoundDown = %d, beklenen 100 (%v)", down.Minor(), err)
	}
	// 100.5 → half-even → 100 (çift)
	he, err := base.MulRate(rate, money.RoundHalfEven)
	if err != nil || he.Minor() != 100 {
		t.Fatalf("RoundHalfEven = %d, beklenen 100 (%v)", he.Minor(), err)
	}
}

func TestRoundHalfEvenTiesGoToEven(t *testing.T) {
	// 101 kuruş × 1.005 = 101.505 → 102 (yarımdan büyük)
	m := money.New(101, money.TRY)
	r, _ := money.RateFromString("1.005")
	got, err := m.MulRate(r, money.RoundHalfEven)
	if err != nil || got.Minor() != 102 {
		t.Fatalf("= %d, beklenen 102 (%v)", got.Minor(), err)
	}
	// tam yarım: 3 kuruş × 1.5 = 4.5 → çifte yuvarla → 4
	got, err = money.New(3, money.TRY).MulRate(mustRate(t, "1.5"), money.RoundHalfEven)
	if err != nil || got.Minor() != 4 {
		t.Fatalf("3 × 1.5 half-even = %d, beklenen 4 (%v)", got.Minor(), err)
	}
	// tam yarım: 5 kuruş × 1.5 = 7.5 → çifte yuvarla → 8
	got, err = money.New(5, money.TRY).MulRate(mustRate(t, "1.5"), money.RoundHalfEven)
	if err != nil || got.Minor() != 8 {
		t.Fatalf("5 × 1.5 half-even = %d, beklenen 8 (%v)", got.Minor(), err)
	}
}

func TestExactDivisionNeedsNoRounding(t *testing.T) {
	m := money.New(100, money.TRY)
	r, _ := money.RateFromString("2")
	for _, mode := range []money.Rounding{money.RoundUp, money.RoundDown, money.RoundHalfEven} {
		got, err := m.MulRate(r, mode)
		if err != nil || got.Minor() != 200 {
			t.Fatalf("mode=%v: %d (%v)", mode, got.Minor(), err)
		}
	}
}

func TestInvalidRoundingModeIsRejected(t *testing.T) {
	m := money.New(100, money.TRY)
	r, _ := money.RateFromString("1.005")
	if _, err := m.MulRate(r, money.Rounding(0)); err == nil {
		t.Fatal("belirtilmemiş yuvarlama yönü hata vermeliydi — sessiz varsayılan yok")
	}
}

func TestNegativeRateIsRejected(t *testing.T) {
	m := money.New(100, money.TRY)
	r, _ := money.RateFromString("-1")
	if _, err := m.MulRate(r, money.RoundUp); err == nil {
		t.Fatal("negatif oran reddedilmeliydi")
	}
	if _, err := m.Convert(money.USD, r, money.RoundUp); err == nil {
		t.Fatal("Convert negatif oranı reddetmeliydi")
	}
}

func TestRoundingString(t *testing.T) {
	cases := map[money.Rounding]string{
		money.RoundUp: "up", money.RoundDown: "down",
		money.RoundHalfEven: "half-even", money.Rounding(99): "invalid",
	}
	for m, want := range cases {
		if got := m.String(); got != want {
			t.Errorf("Rounding(%d).String() = %q, beklenen %q", m, got, want)
		}
	}
}

func mustRate(t *testing.T, s string) money.Rate {
	t.Helper()
	r, err := money.RateFromString(s)
	if err != nil {
		t.Fatalf("RateFromString(%q): %v", s, err)
	}
	return r
}

func TestRoundingNegativeAmounts(t *testing.T) {
	// Negatif tutarlar iade ve düzeltme kayıtlarında oluşur; yuvarlama orada da doğru olmalı.
	neg := money.New(-100, money.TRY)
	r := mustRate(t, "1.005") // -100.5 kuruş

	// RoundUp = sonsuza doğru → -100 (sıfıra yakın)
	up, err := neg.MulRate(r, money.RoundUp)
	if err != nil || up.Minor() != -100 {
		t.Fatalf("negatif RoundUp = %d, beklenen -100 (%v)", up.Minor(), err)
	}
	// RoundDown = sıfıra doğru → -100
	down, err := neg.MulRate(r, money.RoundDown)
	if err != nil || down.Minor() != -100 {
		t.Fatalf("negatif RoundDown = %d, beklenen -100 (%v)", down.Minor(), err)
	}
	// half-even: -100.5 → çifte → -100
	he, err := neg.MulRate(r, money.RoundHalfEven)
	if err != nil || he.Minor() != -100 {
		t.Fatalf("negatif half-even = %d, beklenen -100 (%v)", he.Minor(), err)
	}
	// -101.505 → yarımdan büyük → -102
	he2, err := money.New(-101, money.TRY).MulRate(r, money.RoundHalfEven)
	if err != nil || he2.Minor() != -102 {
		t.Fatalf("negatif half-even (>yarım) = %d, beklenen -102 (%v)", he2.Minor(), err)
	}
	// tam yarım negatif: -3 × 1.5 = -4.5 → çifte → -4
	he3, err := money.New(-3, money.TRY).MulRate(mustRate(t, "1.5"), money.RoundHalfEven)
	if err != nil || he3.Minor() != -4 {
		t.Fatalf("-3 × 1.5 half-even = %d, beklenen -4 (%v)", he3.Minor(), err)
	}
}

func TestOverflowInConversion(t *testing.T) {
	// Çok büyük bir tutar × çok büyük bir oran → int64'e sığmaz, sessizce sarmalamamalı.
	huge := money.New(math.MaxInt64/2, money.TRY)
	big := mustRate(t, "1000000")
	if _, err := huge.MulRate(big, money.RoundUp); !errors.Is(err, money.ErrOverflow) {
		t.Fatalf("taşma ErrOverflow vermeliydi: %v", err)
	}
}
