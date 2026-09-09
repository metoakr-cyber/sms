---
baslik: SMS onayı nedir, nasıl çalışır?
ozet: Bir servise kaydolurken gelen doğrulama kodunun arka planda izlediği yol, kodun neden süreli olduğu ve neden bazen hiç gelmediği.
etiket: Temel
renk: mavi
tarih: 2026-09-09
okuma: 4
---

Bir uygulamaya kaydolurken telefon numaranızı yazarsınız, birkaç saniye sonra
dört ile sekiz haneli bir kod gelir ve onu ekrana girersiniz. Bu adımın adı
**SMS ile doğrulama** (SMS OTP — *one-time password*, tek kullanımlık şifre).

## Neden var?

Bir servis, hesabın arkasında gerçek bir kişi olduğuna dair ucuz bir kanıt
arar. E-posta adresi saniyeler içinde sınırsız sayıda üretilebilir; telefon
numarası üretilemez, çünkü bir operatörden alınır. Servis bu yüzden numarayı
"sahibi olan bir kişi var" göstergesi olarak kullanır.

İkinci bir amaç daha vardır: **hesap kurtarma**. Şifrenizi unuttuğunuzda
numaranıza gönderilen kod, kimliğinizin ikinci kanıtı olur.

## Kod arka planda hangi yolu izler?

1. Uygulamaya numarayı yazarsınız.
2. Servis rastgele bir kod üretir, hesabınızın kaydına **son kullanma
   zamanıyla birlikte** yazar.
3. Kod, servisin anlaşmalı olduğu bir toplu SMS sağlayıcısına verilir.
4. Sağlayıcı mesajı operatör ağına iletir; operatör numaranın bağlı olduğu
   hatta teslim eder.
5. Kodu ekrana girersiniz. Servis, kaydettiği kodla karşılaştırır ve süresi
   dolmamışsa kabul eder.

Zincirin her halkası gecikebilir. Kodun "hemen" gelmemesi çoğu zaman
uygulamanın değil, üçüncü ve dördüncü adımın işidir.

## Kod neden süreli?

Kodun ömrü genellikle birkaç dakikadır. Sebebi basit: kod bir kez ele
geçirildiğinde sonsuza kadar geçerli olsaydı, ekranınızı gören ya da eski bir
mesajı okuyan biri hesabınıza girebilirdi. Süre sınırı, çalınan bir kodun
değerini hızla sıfırlar.

Aynı mantık **tek kullanım** kuralında da geçerlidir: kod bir kez
doğrulandıktan sonra ikinci kez kabul edilmez.

## Kod neden bazen hiç gelmez?

Bunun tek bir sebebi yok. En sık görülenler:

- **Operatör filtresi.** Toplu mesaj trafiği spam olarak sınıflanıp
  düşürülebilir.
- **Numara tipi kısıtı.** Bazı servisler yalnız mobil numaraları kabul eder;
  sabit hat ya da bazı sanal numara aralıklarını reddeder.
- **Servis tarafında hız sınırı.** Kısa sürede çok istek gönderildiyse servis
  yeni kod üretmeyi geçici olarak durdurur.
- **Ülke uyuşmazlığı.** Servis, hesabın açıldığı ülkeyle numaranın ülkesini
  karşılaştırıp reddedebilir.

## Bizde ne oluyor?

Onay360'ta bir numara aldığınızda numaranın ne kadar süre geçerli olduğu
**sağlayıcıdan gelen bitiş zamanına** göre gösterilir; ekrandaki sayaç
tarayıcınızın değil, sunucunun saatinden hesaplanır. Süre kod gelmeden
dolarsa ücret bakiyenize otomatik iade edilir — bunun için talep açmanız
gerekmez.
