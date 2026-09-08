.DEFAULT_GOAL := help
SHELL := /bin/bash
API := api
WEB := web
COMPOSE := docker compose -f deploy/docker-compose.dev.yml

# .env HER HEDEFTE yüklenir.
#
# `migrate-up` DATABASE_URL'i ortamdan bekliyordu ve .env'i okumuyordu; dosya
# oracıkta dururken "database= bağlanılamadı" hatası veriyordu. Aynı hatanın
# Go tarafındaki eşi config.LoadDotEnv içindeydi (bkz. docs/memory.md §3.13).
# Kural: bir aracı çalıştırmak için önce elle `export` gerekiyorsa, o araç
# bozuktur.
ifneq (,$(wildcard .env))
include .env
export
endif

# TESTLER AYRI VERİTABANI KULLANIR.
#
# Aynı veritabanını paylaşmak, `make check` koştuğunuzda geliştirme
# hesabınızın, sağlayıcı kaydınızın ve tüm katalogunuzun silinmesi demekti
# (entegrasyon testleri `DELETE FROM providers` yapıyor). Bu tek bir oturumda
# beş kez yaşandı; her seferinde katalog yeniden senkronlandı.
TEST_DATABASE_URL ?= $(subst /smsplatform?,/smsplatform_test?,$(DATABASE_URL))
export TEST_DATABASE_URL

## help: bu listeyi göster
help:
	@grep -E '^## ' $(MAKEFILE_LIST) | sed 's/## //' | awk -F': ' '{printf "  \033[36m%-18s\033[0m %s\n", $$1, $$2}'

## tools: geliştirme araçlarını kur (sqlc, goose, golangci-lint, air)
tools:
	go install github.com/sqlc-dev/sqlc/cmd/sqlc@latest
	go install github.com/pressly/goose/v3/cmd/goose@latest
	go install github.com/golangci/golangci-lint/v2/cmd/golangci-lint@latest

## up: postgres + redis başlat
up:
	$(COMPOSE) up -d
	@echo "postgres :5432  redis :6379"

## down: altyapıyı durdur
down:
	$(COMPOSE) down

## reset: altyapıyı sıfırla (VERİ SİLİNİR)
reset:
	$(COMPOSE) down -v && $(COMPOSE) up -d

## db-test: testler için AYRI veritabanı kur (yoksa oluşturur, migration'ları uygular)
db-test:
	@docker exec smsplatform-dev-postgres-1 psql -U smsplatform -d postgres -tAc \
	  "SELECT 1 FROM pg_database WHERE datname='smsplatform_test'" | grep -q 1 \
	  || docker exec smsplatform-dev-postgres-1 createdb -U smsplatform smsplatform_test
	@cd $(API) && goose -dir migrations postgres "$(TEST_DATABASE_URL)" up

## migrate-up: migration'ları uygula
migrate-up:
	cd $(API) && goose -dir migrations postgres "$$DATABASE_URL" up

## migrate-down: son migration'ı geri al
migrate-down:
	cd $(API) && goose -dir migrations postgres "$$DATABASE_URL" down

## migrate-new: yeni migration dosyası (make migrate-new name=add_x)
migrate-new:
	cd $(API) && goose -dir migrations create $(name) sql

## gen: sqlc kodunu üret
gen:
	cd $(API) && sqlc generate

## gen-check: üretilen kod güncel mi (CI)
gen-check:
	cd $(API) && sqlc diff

## dev: her şeyi başlat (altyapı + api)
dev: up
	cd $(API) && go run ./cmd/server

## worker: arka plan işçilerini başlat
worker:
	cd $(API) && go run ./cmd/worker

## test: tüm testler
test:
	cd $(API) && go test ./... -race -count=1

## test-cover: kapsam raporu
test-cover:
	cd $(API) && go test ./... -race -coverprofile=coverage.out -covermode=atomic && go tool cover -func=coverage.out | tail -1

## test-integration: gerçek Postgres'e karşı entegrasyon testleri
# -p 1 ZORUNLU: bu testler TEK bir gerçek veritabanını paylaşıyor ve her paket
# kendi kurulumunda katalog tablolarını temizliyor. Paralel koşarlarsa
# birbirlerinin verisini silerler ve testler RASTGELE kırılır.
test-integration:
	cd $(API) && go test -tags=integration -p 1 ./... -race -count=1

## smoke: uçtan uca duman testi (sunucuyu başlatır, senaryoları koşar, temizler)
smoke:
	./scripts/smoke-auth.sh

## lint: statik analiz
lint:
	cd $(API) && go vet ./... && golangci-lint run

## responsive: mobil/tarayıcı denetimi (her iki sunucu ayakta olmalı)
responsive:
	cd web && npm run audit:responsive

## commit: kontroller geçerse commit eder (make commit m="mesaj")
commit:
	./scripts/commit.sh "$(m)"

## check: birleştirmeden önce çalıştır
check:
	./scripts/check.sh

.PHONY: db-test responsive commit help tools up down reset migrate-up migrate-down migrate-new gen gen-check dev worker test test-cover test-integration smoke lint check
