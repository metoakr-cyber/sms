package http

import apperr "github.com/ikmetrik/sms-platform/api/internal/domain/errors"

var notFound = apperr.New(apperr.KindNotFound, "ROUTE_NOT_FOUND", "İstenen adres bulunamadı.")
