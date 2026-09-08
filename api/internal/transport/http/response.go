package http

import (
	"log/slog"
	"net/http"

	"github.com/gin-gonic/gin"

	apperr "github.com/ikmetrik/sms-platform/api/internal/domain/errors"
	"github.com/ikmetrik/sms-platform/api/internal/transport/http/middleware"
)

// ErrorBody tüm hata yanıtlarının değişmez biçimi (docs/design.md §11).
type ErrorBody struct {
	Error ErrorDetail `json:"error"`
}

type ErrorDetail struct {
	Code      string `json:"code"`
	Message   string `json:"message"`
	RequestID string `json:"requestId"`
}

// Fail bir hatayı istemciye yazar.
//
// Ham hata mesajı ASLA gövdeye konmaz — eski prototip err.Message'ı doğrudan
// HTML'e basıyordu (yansımalı XSS + bilgi sızıntısı). Burada yalnız katalogdaki
// Türkçe metin gider; gerçek sebep log'a yazılır.
func Fail(c *gin.Context, err error) {
	reqID := middleware.RequestIDFrom(c.Request.Context())

	appErr, ok := apperr.As(err)
	if !ok {
		appErr = apperr.Internal(err)
	}

	status := appErr.HTTPStatus()

	attrs := []any{
		"request_id", reqID,
		"code", appErr.Code,
		"status", status,
		"path", c.FullPath(),
	}
	if inner := appErr.Unwrap(); inner != nil {
		attrs = append(attrs, "cause", inner.Error())
	}
	if status >= http.StatusInternalServerError {
		slog.Error("istek başarısız", attrs...)
	} else {
		slog.Warn("istek reddedildi", attrs...)
	}

	c.AbortWithStatusJSON(status, ErrorBody{Error: ErrorDetail{
		Code:      appErr.Code,
		Message:   appErr.Message,
		RequestID: reqID,
	}})
}

// OK başarılı bir yanıt yazar.
func OK(c *gin.Context, body any) { c.JSON(http.StatusOK, body) }

// Created 201 yanıtı yazar.
func Created(c *gin.Context, body any) { c.JSON(http.StatusCreated, body) }

// NoContent 204 yanıtı yazar.
func NoContent(c *gin.Context) { c.Status(http.StatusNoContent) }
