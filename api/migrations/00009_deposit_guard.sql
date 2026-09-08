-- +goose Up
-- +goose StatementBegin

-- ── BAKİYE YÜKLEME DURUM MAKİNESİ KORUMASI ──
--
-- `orders` tarafında geçiş koruması hem kodda (domain/order) hem de
-- veritabanında (orders_guard_transition) vardı; `deposits` tarafında ise
-- HİÇBİR DB koruması yoktu. Bu, servis katmanının atlandığı her yolda
-- (elle psql, ileride yazılacak bir arka plan işi, bir veri düzeltme betiği)
-- COMPLETED bir talebin PENDING'e çekilip İKİNCİ KEZ onaylanabilmesi
-- demekti — gerçek para kaybı.
--
-- İki katman gerekli: Go katmanı (internal/domain/deposit) hatayı kullanıcıya
-- anlaşılır kılar, tetikleyici ise son savunmadır.
--
-- İzinli geçişler (docs/design.md §7.2):
--   PENDING   → COMPLETED | REJECTED
--   COMPLETED → REFUNDED            (v1'de uç nokta YOK; graf ileride bozulmasın diye tanımlı)
--
-- test: deposit_integration_test.go#TestTerminalDepositCannotChangeStatus
CREATE OR REPLACE FUNCTION deposits_guard_transition() RETURNS trigger AS $$
BEGIN
    IF OLD.status = NEW.status THEN
        RETURN NEW;
    END IF;

    IF OLD.status IN ('REJECTED', 'REFUNDED') THEN
        RAISE EXCEPTION
            'gecersiz yukleme gecisi: % -> % (terminal durumdan cikis yok)',
            OLD.status, NEW.status;
    END IF;

    IF NOT (
        (OLD.status = 'PENDING'   AND NEW.status IN ('COMPLETED', 'REJECTED')) OR
        (OLD.status = 'COMPLETED' AND NEW.status = 'REFUNDED')
    ) THEN
        RAISE EXCEPTION 'gecersiz yukleme gecisi: % -> %', OLD.status, NEW.status;
    END IF;

    RETURN NEW;
END;
$$ LANGUAGE plpgsql;
-- +goose StatementEnd

-- +goose StatementBegin
CREATE TRIGGER deposits_guard_transition_trg
    BEFORE UPDATE ON deposits FOR EACH ROW EXECUTE FUNCTION deposits_guard_transition();
-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin
DROP TRIGGER IF EXISTS deposits_guard_transition_trg ON deposits;
DROP FUNCTION IF EXISTS deposits_guard_transition();
-- +goose StatementEnd
