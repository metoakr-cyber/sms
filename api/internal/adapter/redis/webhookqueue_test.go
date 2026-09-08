package redis

import (
	"testing"
	"time"
)

// TestDequeueRespectsRedisMinimumTimeout
//
// Redis'in en küçük bloklama süresi 1 saniyedir. Daha kısa bir değer sessizce
// yuvarlanmaz: istemci her çağrıda uyarı basar ve saniyede bir koşan bir
// tüketicide bu, gerçek olayları boğan bir gürültü akıntısı olur.
//
// Sınır burada, arka ucun sahibi olan katmanda korunur — çağıranın
// hatırlamasına bırakılmaz.
func TestDequeueRespectsRedisMinimumTimeout(t *testing.T) {
	cases := []struct {
		in, want time.Duration
	}{
		{0, time.Second},
		{900 * time.Millisecond, time.Second},
		{time.Second, time.Second},
		{2 * time.Second, 2 * time.Second},
		{-1, time.Second},
	}
	for _, c := range cases {
		if got := clampBlock(c.in); got != c.want {
			t.Errorf("clampBlock(%v) = %v, %v bekleniyordu", c.in, got, c.want)
		}
	}
}
