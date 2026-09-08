package money_test

import (
	"errors"
	"math"
	"testing"

	"github.com/ikmetrik/sms-platform/api/internal/domain/money"
)

func TestNewAndAccessors(t *testing.T) {
	m := money.New(1250, money.TRY)
	if got := m.Minor(); got != 1250 {
		t.Fatalf("Minor() = %d, beklenen 1250", got)
	}
	if m.Currency() != money.TRY {
		t.Fatalf("Currency() = %v, beklenen TRY", m.Currency())
	}
	if m.IsZero() || m.IsNegative() || !m.IsPositive() {
		t.Fatal("işaret yüklemleri hatalı")
	}
}

func TestNewPanicsOnInvalidCurrency(t *testing.T) {
	defer func() {
		if recover() == nil {
			t.Fatal("geçersiz para biriminde panic bekleniyordu")
		}
	}()
	money.New(1, money.Currency{})
}

func TestAddSubSameCurrency(t *testing.T) {
	a, b := money.New(1250, money.TRY), money.New(750, money.TRY)

	sum, err := a.Add(b)
	if err != nil || sum.Minor() != 2000 {
		t.Fatalf("Add = %v, %v; beklenen 2000", sum.Minor(), err)
	}
	diff, err := a.Sub(b)
	if err != nil || diff.Minor() != 500 {
		t.Fatalf("Sub = %v, %v; beklenen 500", diff.Minor(), err)
	}
	// Negatife düşme aritmetik olarak serbesttir; yasak veritabanı ve cüzdan
	// katmanında uygulanır (CHECK balance_minor >= 0).
	neg, err := b.Sub(a)
	if err != nil || neg.Minor() != -500 {
		t.Fatalf("Sub negatif = %v, %v; beklenen -500", neg.Minor(), err)
	}
}

func TestCurrencyMismatchIsRejected(t *testing.T) {
	try, usd := money.New(100, money.TRY), money.New(100, money.USD)
	if _, err := try.Add(usd); !errors.Is(err, money.ErrCurrencyMismatch) {
		t.Fatalf("Add farklı birim: %v, ErrCurrencyMismatch bekleniyordu", err)
	}
	if _, err := try.Sub(usd); !errors.Is(err, money.ErrCurrencyMismatch) {
		t.Fatalf("Sub farklı birim: %v", err)
	}
	if _, err := try.Cmp(usd); !errors.Is(err, money.ErrCurrencyMismatch) {
		t.Fatalf("Cmp farklı birim: %v", err)
	}
}

func TestOverflowIsDetectedNotWrapped(t *testing.T) {
	max := money.New(math.MaxInt64, money.TRY)
	if _, err := max.Add(money.New(1, money.TRY)); !errors.Is(err, money.ErrOverflow) {
		t.Fatalf("taşma yakalanmadı: %v", err)
	}
	min := money.New(math.MinInt64, money.TRY)
	if _, err := min.Add(money.New(-1, money.TRY)); !errors.Is(err, money.ErrOverflow) {
		t.Fatalf("negatif taşma yakalanmadı: %v", err)
	}
	if _, err := min.Neg(); !errors.Is(err, money.ErrOverflow) {
		t.Fatalf("MinInt64 negasyonu taşma vermeliydi: %v", err)
	}
}

func TestCmpAndMax(t *testing.T) {
	a, b := money.New(100, money.TRY), money.New(200, money.TRY)
	if c, _ := a.Cmp(b); c != -1 {
		t.Fatalf("Cmp(a,b) = %d, beklenen -1", c)
	}
	if c, _ := b.Cmp(a); c != 1 {
		t.Fatalf("Cmp(b,a) = %d, beklenen 1", c)
	}
	if c, _ := a.Cmp(money.New(100, money.TRY)); c != 0 {
		t.Fatalf("Cmp eşit = %d, beklenen 0", c)
	}
	m, err := money.Max(a, b)
	if err != nil || m.Minor() != 200 {
		t.Fatalf("Max = %v, %v; beklenen 200", m.Minor(), err)
	}
}

// Bu test, mevcut prototipteki float aritmetiğinin neden kabul edilemez olduğunu
// somut olarak gösterir: 0.1 + 0.2 float64'te 0.30000000000000004'tür.
func TestNoFloatDrift(t *testing.T) {
	ten := money.New(10, money.TRY) // 0,10 ₺
	twenty := money.New(20, money.TRY)
	sum, err := ten.Add(twenty)
	if err != nil {
		t.Fatal(err)
	}
	if sum.Minor() != 30 {
		t.Fatalf("0,10 + 0,20 = %d kuruş, beklenen tam 30", sum.Minor())
	}

	// Bin kez 0,01 ekle → tam olarak 10,00 ₺ olmalı, 9,99 veya 10,01 değil.
	acc := money.Zero(money.TRY)
	for i := 0; i < 1000; i++ {
		acc, err = acc.Add(money.New(1, money.TRY))
		if err != nil {
			t.Fatal(err)
		}
	}
	if acc.Minor() != 1000 {
		t.Fatalf("1000 × 0,01 = %d kuruş, beklenen tam 1000", acc.Minor())
	}
}

func TestString(t *testing.T) {
	tests := []struct {
		m    money.Money
		want string
	}{
		{money.New(1250, money.TRY), "12,50 TRY"},
		{money.New(-1250, money.TRY), "-12,50 TRY"},
		{money.New(5, money.TRY), "0,05 TRY"},
		{money.New(0, money.TRY), "0,00 TRY"},
		{money.New(350000, money.USD), "0,350000 USD"}, // mikro-dolar, ADR-026
	}
	for _, tt := range tests {
		if got := tt.m.String(); got != tt.want {
			t.Errorf("String() = %q, beklenen %q", got, tt.want)
		}
	}
	if got := (money.Money{}).String(); got != "Money(<geçersiz>)" {
		t.Errorf("sıfır değeri String() = %q", got)
	}
}

func TestFromISO(t *testing.T) {
	if c, err := money.FromISO(money.ISOUSD); err != nil || c != money.USD {
		t.Fatalf("FromISO(840) = %v, %v", c, err)
	}
	if _, err := money.FromISO(money.ISOEUR); !errors.Is(err, money.ErrUnsupportedCurrency) {
		t.Fatalf("EUR desteklenmemeli: %v", err)
	}
}
