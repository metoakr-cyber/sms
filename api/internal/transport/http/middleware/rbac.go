package middleware

import (
	"github.com/gin-gonic/gin"

	apperr "github.com/ikmetrik/sms-platform/api/internal/domain/errors"
)

// RequirePermission belirli bir izni zorunlu kılar.
//
// Yetkilendirme İKİ katmanlıdır ve ikisi de zorunludur:
//  1. İzin kontrolü (burası) — "bu kullanıcı bu İŞLEMİ yapabilir mi"
//  2. Sahiplik kontrolü (sorgunun içinde) — "bu KAYIT bu kullanıcının mı"
//
// Eski prototipteki üç kritik açık (ödeme onayı, profil güncelleme, sipariş
// sorgulama) tam olarak bu iki katmanın eksikliğinden doğuyordu
// (docs/memory.md §3.3).
func RequirePermission(perm string, fail func(*gin.Context, error)) gin.HandlerFunc {
	return func(c *gin.Context) {
		if !HasPermission(c, perm) {
			fail(c, apperr.ErrForbidden)
			return
		}
		c.Next()
	}
}

// RequireAnyPermission verilen izinlerden en az birini arar.
func RequireAnyPermission(fail func(*gin.Context, error), perms ...string) gin.HandlerFunc {
	return func(c *gin.Context) {
		for _, p := range perms {
			if HasPermission(c, p) {
				c.Next()
				return
			}
		}
		fail(c, apperr.ErrForbidden)
	}
}

// HasPermission kullanıcının izni olup olmadığını söyler.
func HasPermission(c *gin.Context, perm string) bool {
	v, ok := c.Get(CtxPermissions)
	if !ok {
		return false
	}
	perms, ok := v.([]string)
	if !ok {
		return false
	}
	for _, p := range perms {
		if p == perm {
			return true
		}
	}
	return false
}
