#!/usr/bin/env python3
"""Yorumda verilen garantilerin teste bağlı olmasını zorunlu kılar.

NEDEN VAR
─────────
Kod incelemesinde beş ayrı yerde şu örüntü bulundu: bir yorum bir GARANTİ
iddia ediyor ama kod o garantiyi sağlamıyor.

    // aynı istek tekrarlanırsa bakiye iki kez değişmez  → her tekrar yeni UUID
    // Ama log'larız                                     → gövdede hiç log yok
    // config üretimde bu sağlayıcıyı reddeder           → main.go config'i okumuyor
    // JavaScript okuyamaz — XSS ile çalınamaz           → aynı değer JSON'da dönüyor

Bu, tekil hatalardan DAHA TEHLİKELİDİR: kodu okuyan (insan veya model) yorumu
okuyup kontrolün yapıldığını varsayar ve inceleme orada durur. Beş vakanın
hepsinde yorum DOĞRU tasarımı tarif ediyordu — tasarım biliniyordu, sadece
koda bağlanmamıştı.

KURAL
─────
Bir yorum bloğu garanti sözcüğü içeriyorsa, aynı blokta o garantiyi doğrulayan
bir test referansı bulunmalıdır:

    // test: pricing_test.go#TestKK304_GoldenPrice

Referans biçimi serbesttir; amaç okuyanı testin YERİNE götürmektir.
"""
import re
import sys
from pathlib import Path

ROOT = Path(__file__).resolve().parent.parent

# Garanti iddia eden ifadeler. Liste kasıtlı olarak DAR: her yorumu değil,
# mutlak bir söz veren yorumları yakalamak istiyoruz.
GUARANTEE = re.compile(
    r"(asla\b|ASLA\b|hiçbir zaman|imkânsız|imkansız|"
    r"iki kez (değişmez|uygulanmaz|yazılmaz)|"
    r"reddeder|reddedilir|engellenir|engeller\b|"
    r"log'larız|loglariz|"
    r"okuyamaz|çalınamaz|calinamaz|"
    r"garanti eder|zorlanır|zorlar\b|"
    # YASAK BİLDİREN KALIPLAR. Bunlar listede yoktu ve gerçek bir hatayı
    # kaçırdılar: smoke-auth.sh "TRUNCATE ... CASCADE KULLANILMAZ" diyordu,
    # koddaysa tam olarak o vardı; cascade tohum verisini siliyor, uygulama
    # NO_PRICING_RULE ile düşüyordu. "X yapılmaz" da en az "asla" kadar
    # bağlayıcı bir sözdür.
    r"kullanılmaz|kullanilmaz|yapılmaz|yapilmaz|"
    r"saklanmaz|gönderilmez|gonderilmez|sızmaz|sizmaz|sızamaz|sizamaz)",
    re.IGNORECASE,
)
TEST_REF = re.compile(r"test:\s*\S+", re.IGNORECASE)

# Referansın parçaları:  test: yol/dosya_test.go#TestAdi   (yol isteğe bağlı)
TEST_REF_PARTS = re.compile(r"test:\s*([^\s#]+)?(?:#(\w+))?", re.IGNORECASE)

# Go test fonksiyonu tanımı.
GO_TEST_DEF = re.compile(r"^func\s+(Test\w+)\s*\(", re.MULTILINE)

# Kabuk betiklerinde test fonksiyonu / etiketi.
SH_TEST_DEF = re.compile(r"^(?:function\s+)?(\w+)\s*\(\)", re.MULTILINE)

# Bu dosyalar taranmaz: üretilen kod, testin kendisi, bu betik.
SKIP = ("internal/db/", "_test.go", "check-guarantees.py", "/node_modules/", "/.git/")


def comment_blocks(path: Path):
    """Ardışık yorum satırlarını blok olarak döner: (başlangıç_satırı, metin)."""
    prefix = "#" if path.suffix in (".sh", ".py", ".yml", ".yaml") else "//"
    lines = path.read_text(encoding="utf-8", errors="replace").splitlines()
    block, start = [], 0
    for i, raw in enumerate(lines, 1):
        s = raw.strip()
        is_comment = s.startswith(prefix) or (prefix == "//" and s.startswith("--"))
        if is_comment:
            if not block:
                start = i
            block.append(s)
        elif block:
            yield start, "\n".join(block)
            block = []
    if block:
        yield start, "\n".join(block)


def index_tests():
    """Depodaki tüm test adlarını ve hangi dosyada tanımlı olduklarını çıkarır."""
    by_name: dict[str, set[str]] = {}
    files: set[str] = set()
    for pattern in ("api/**/*_test.go", "scripts/*_test.sh", "web/**/*.spec.ts",
                    "web/**/*.test.ts"):
        for f in ROOT.glob(pattern):
            if "/node_modules/" in str(f):
                continue
            rel = str(f.relative_to(ROOT))
            files.add(rel)
            body = f.read_text(encoding="utf-8", errors="replace")
            pat = GO_TEST_DEF if f.suffix == ".go" else SH_TEST_DEF
            for name in pat.findall(body):
                by_name.setdefault(name, set()).add(rel)
            # Playwright/vitest:  test("ad") / it("ad")
            for name in re.findall(r"""(?:^|\s)(?:test|it)\(\s*['"`]([^'"`]+)""", body):
                by_name.setdefault(name, set()).add(rel)
    return by_name, files


def check_ref(ref: str, by_name, files) -> str | None:
    """Referans gerçek bir teste işaret ediyor mu? Hata metnini ya da None döner."""
    m = TEST_REF_PARTS.search(ref)
    if not m:
        return None
    path, name = m.group(1), m.group(2)
    if name:
        where = by_name.get(name)
        if not where:
            return f"'{name}' adında bir test YOK"
        if path:
            tail = path.lstrip("./").replace("../", "")
            if tail and not any(w.endswith(tail) or tail.endswith(w) for w in where):
                return (f"'{name}' testi {path} içinde değil "
                        f"(bulunduğu yer: {', '.join(sorted(where))})")
        return None
    if path and path.endswith((".go", ".sh", ".ts")):
        tail = path.lstrip("./").replace("../", "")
        if any(f.endswith(tail) for f in files):
            return None
        # Test indeksinde değil ama diskte olabilir: duman betikleri
        # (smoke-*.sh) ve sözleşme testi yardımcıları (contract.go) test
        # adı taşımaz ama gerçek doğrulama dosyalarıdır.
        if any(ROOT.glob("**/" + tail)):
            return None
        return f"'{path}' diye bir dosya YOK"
    return None


def main() -> int:
    by_name, test_files = index_tests()
    targets = []
    for pattern in ("api/**/*.go", "scripts/*.sh", "api/**/*.sql"):
        targets.extend(ROOT.glob(pattern))

    violations = []
    # 🔴 SAHTE REFERANSLAR. Yalnız "test:" yazısının VARLIĞINI aramak yeterli
    # değildi: var olmayan bir teste işaret eden bir referans, denetimi
    # geçerken garantiyi dayanaksız bırakır — denetleyicinin önlemek için
    # yazıldığı hatanın ta kendisi.
    dangling = []
    for f in sorted(set(targets)):
        rel = str(f.relative_to(ROOT))
        if any(s in rel for s in SKIP):
            continue
        for line, text in comment_blocks(f):
            for ref in TEST_REF.findall(text):
                if problem := check_ref(ref, by_name, test_files):
                    dangling.append((rel, line, problem))
            if GUARANTEE.search(text) and not TEST_REF.search(text):
                first = next(
                    (l for l in text.splitlines() if GUARANTEE.search(l)), text
                )
                violations.append((rel, line, first.strip()))

    if dangling:
        print(f"  ✗ {len(dangling)} test referansı GERÇEK OLMAYAN bir teste işaret ediyor:\n")
        for rel, line, problem in dangling:
            print(f"    {rel}:{line}")
            print(f"      {problem}")
        print("\n  Var olmayan bir teste yapılan atıf, garantiyi dayanaksız bırakır\n"
              "  ve denetimi sessizce geçer. Testi yazın ya da atfı düzeltin.")
        return 1

    if not violations:
        print(f"  ✓ garanti yorumları teste bağlı ve atıflar gerçek "
              f"({len(set(targets))} dosya, {len(by_name)} test)")
        return 0

    print(f"  ✗ {len(violations)} garanti yorumu teste bağlı değil:\n")
    for rel, line, snippet in violations:
        print(f"    {rel}:{line}")
        print(f"      {snippet[:110]}")
    print(
        "\n  Bir yorum garanti veriyorsa, aynı blokta o garantiyi doğrulayan bir\n"
        "  test referansı bulunmalı:   // test: dosya_test.go#TestAdi\n"
        "  Test yoksa ya testi yazın ya da yorumu garanti vermeyecek şekilde düzeltin."
    )
    return 1


if __name__ == "__main__":
    sys.exit(main())
