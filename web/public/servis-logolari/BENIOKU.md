# Servis logoları

Buraya servis logolarını koyun. Dosya adı serbesttir ama servis koduyla
eşleşmesi işleri kolaylaştırır:

    wa.svg    → Whatsapp
    tg.svg    → Telegram
    ig.svg    → Instagram
    fb.svg    → Facebook
    go.svg    → Google
    am.svg    → Amazon

**Biçim:** SVG tercih edilir (her ölçekte net, dosya küçük). PNG kullanılacaksa
en az 128×128 ve şeffaf arka planlı olsun.

**Koyu tema uyarısı:** Logo koyu zemin üzerinde görünecek. Siyah tek renkli
logolar koyu temada kaybolur; beyaz veya renkli sürümünü kullanın.

Dosyayı koyduktan sonra veritabanına tanıtın:

    cd api && go run ./cmd/cli catalog:icon --service=wa --url=/servis-logolari/wa.svg

Tanıtılmayan servisler harf rozetiyle gösterilir — kırık ikon çıkmaz.
