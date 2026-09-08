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
    r"garanti eder|zorlanır|zorlar\b)",
    re.IGNORECASE,
)
TEST_REF = re.compile(r"test:\s*\S+", re.IGNORECASE)

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


def main() -> int:
    targets = []
    for pattern in ("api/**/*.go", "scripts/*.sh", "api/**/*.sql"):
        targets.extend(ROOT.glob(pattern))

    violations = []
    for f in sorted(set(targets)):
        rel = str(f.relative_to(ROOT))
        if any(s in rel for s in SKIP):
            continue
        for line, text in comment_blocks(f):
            if GUARANTEE.search(text) and not TEST_REF.search(text):
                first = next(
                    (l for l in text.splitlines() if GUARANTEE.search(l)), text
                )
                violations.append((rel, line, first.strip()))

    if not violations:
        print(f"  ✓ garanti yorumlarının tümü teste bağlı ({len(set(targets))} dosya tarandı)")
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
