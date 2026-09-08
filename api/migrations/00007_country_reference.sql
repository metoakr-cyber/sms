-- +goose Up
-- +goose StatementBegin

-- ÜLKE REFERANSI
--
-- NEDEN VAR
-- ─────────
-- Katalog senkronu `countries.iso2` alanına SAĞLAYICININ ülke kodunu yazıyordu
-- (`iso = rc.RemoteCode`). FakeProvider ISO2 kullandığı için bu hata görünmedi;
-- HeroSMS sayısal kimlik kullanıyor ve veritabanında `iso2 = '62'` gibi satırlar
-- oluştu. Sonuç: Türkçe ülke adı yok, telefon kodu yok ve sütun kendi
-- sözleşmesini ihlal ediyor.
--
-- Sağlayıcının kodu `provider_dimension_maps` tablosuna aittir; `countries.iso2`
-- ISO 3166-1 alpha-2 OLMALIDIR. İkisini karıştırmak, ikinci bir sağlayıcı
-- eklendiğinde aynı ülkenin iki satır olarak açılmasına yol açardı.
--
-- Bu tablo İNGİLİZCE ADI ISO2'ye çevirir. Ad bir kimlik değildir ama
-- sağlayıcıların ortak paydası budur; takma ad tablosu yazım farklarını kapatır.
CREATE TABLE country_reference (
    name_key    TEXT PRIMARY KEY,          -- küçük harfe indirgenmiş sağlayıcı adı
    iso2        TEXT NOT NULL,
    name_en     TEXT NOT NULL,
    name_tr     TEXT NOT NULL,
    phone_code  TEXT NOT NULL
);

CREATE INDEX country_reference_iso2_idx ON country_reference (iso2);

INSERT INTO country_reference (name_key, iso2, name_en, name_tr, phone_code) VALUES
('ukraine', 'UA', 'Ukraine', 'Ukrayna', '380'),
('russia', 'RU', 'Russia', 'Rusya', '7'),
('kazakhstan', 'KZ', 'Kazakhstan', 'Kazakistan', '7'),
('china', 'CN', 'China', 'Çin', '86'),
('philippines', 'PH', 'Philippines', 'Filipinler', '63'),
('myanmar', 'MM', 'Myanmar', 'Myanmar', '95'),
('indonesia', 'ID', 'Indonesia', 'Endonezya', '62'),
('malaysia', 'MY', 'Malaysia', 'Malezya', '60'),
('kenya', 'KE', 'Kenya', 'Kenya', '254'),
('tanzania', 'TZ', 'Tanzania', 'Tanzanya', '255'),
('vietnam', 'VN', 'Vietnam', 'Vietnam', '84'),
('kyrgyzstan', 'KG', 'Kyrgyzstan', 'Kırgızistan', '996'),
('israel', 'IL', 'Israel', 'İsrail', '972'),
('hong kong', 'HK', 'Hong Kong', 'Hong Kong', '852'),
('poland', 'PL', 'Poland', 'Polonya', '48'),
('united kingdom', 'GB', 'United Kingdom', 'Birleşik Krallık', '44'),
('madagascar', 'MG', 'Madagascar', 'Madagaskar', '261'),
('dr congo', 'CD', 'DR Congo', 'Demokratik Kongo Cumhuriyeti', '243'),
('nigeria', 'NG', 'Nigeria', 'Nijerya', '234'),
('macao', 'MO', 'Macao', 'Makao', '853'),
('egypt', 'EG', 'Egypt', 'Mısır', '20'),
('india', 'IN', 'India', 'Hindistan', '91'),
('ireland', 'IE', 'Ireland', 'İrlanda', '353'),
('cambodia', 'KH', 'Cambodia', 'Kamboçya', '855'),
('laos', 'LA', 'Laos', 'Laos', '856'),
('haiti', 'HT', 'Haiti', 'Haiti', '509'),
('ivory coast', 'CI', 'Ivory Coast', 'Fildişi Sahili', '225'),
('gambia', 'GM', 'Gambia', 'Gambiya', '220'),
('serbia', 'RS', 'Serbia', 'Sırbistan', '381'),
('yemen', 'YE', 'Yemen', 'Yemen', '967'),
('south africa', 'ZA', 'South Africa', 'Güney Afrika', '27'),
('romania', 'RO', 'Romania', 'Romanya', '40'),
('colombia', 'CO', 'Colombia', 'Kolombiya', '57'),
('estonia', 'EE', 'Estonia', 'Estonya', '372'),
('azerbaijan', 'AZ', 'Azerbaijan', 'Azerbaycan', '994'),
('canada', 'CA', 'Canada', 'Kanada', '1'),
('morocco', 'MA', 'Morocco', 'Fas', '212'),
('ghana', 'GH', 'Ghana', 'Gana', '233'),
('argentina', 'AR', 'Argentina', 'Arjantin', '54'),
('uzbekistan', 'UZ', 'Uzbekistan', 'Özbekistan', '998'),
('cameroon', 'CM', 'Cameroon', 'Kamerun', '237'),
('chad', 'TD', 'Chad', 'Çad', '235'),
('germany', 'DE', 'Germany', 'Almanya', '49'),
('lithuania', 'LT', 'Lithuania', 'Litvanya', '370'),
('croatia', 'HR', 'Croatia', 'Hırvatistan', '385'),
('sweden', 'SE', 'Sweden', 'İsveç', '46'),
('iraq', 'IQ', 'Iraq', 'Irak', '964'),
('netherlands', 'NL', 'Netherlands', 'Hollanda', '31'),
('latvia', 'LV', 'Latvia', 'Letonya', '371'),
('austria', 'AT', 'Austria', 'Avusturya', '43'),
('belarus', 'BY', 'Belarus', 'Belarus', '375'),
('thailand', 'TH', 'Thailand', 'Tayland', '66'),
('saudi arabia', 'SA', 'Saudi Arabia', 'Suudi Arabistan', '966'),
('mexico', 'MX', 'Mexico', 'Meksika', '52'),
('taiwan', 'TW', 'Taiwan', 'Tayvan', '886'),
('spain', 'ES', 'Spain', 'İspanya', '34'),
('iran', 'IR', 'Iran', 'İran', '98'),
('algeria', 'DZ', 'Algeria', 'Cezayir', '213'),
('slovenia', 'SI', 'Slovenia', 'Slovenya', '386'),
('bangladesh', 'BD', 'Bangladesh', 'Bangladeş', '880'),
('senegal', 'SN', 'Senegal', 'Senegal', '221'),
('turkey', 'TR', 'Turkey', 'Türkiye', '90'),
('czech', 'CZ', 'Czech', 'Çekya', '420'),
('sri lanka', 'LK', 'Sri Lanka', 'Sri Lanka', '94'),
('peru', 'PE', 'Peru', 'Peru', '51'),
('pakistan', 'PK', 'Pakistan', 'Pakistan', '92'),
('new zealand', 'NZ', 'New Zealand', 'Yeni Zelanda', '64'),
('guinea', 'GN', 'Guinea', 'Gine', '224'),
('mali', 'ML', 'Mali', 'Mali', '223'),
('venezuela', 'VE', 'Venezuela', 'Venezuela', '58'),
('ethiopia', 'ET', 'Ethiopia', 'Etiyopya', '251'),
('mongolia', 'MN', 'Mongolia', 'Moğolistan', '976'),
('brazil', 'BR', 'Brazil', 'Brezilya', '55'),
('afghanistan', 'AF', 'Afghanistan', 'Afganistan', '93'),
('uganda', 'UG', 'Uganda', 'Uganda', '256'),
('angola', 'AO', 'Angola', 'Angola', '244'),
('cyprus', 'CY', 'Cyprus', 'Kıbrıs', '357'),
('france', 'FR', 'France', 'Fransa', '33'),
('papua', 'PG', 'Papua', 'Papua Yeni Gine', '675'),
('mozambique', 'MZ', 'Mozambique', 'Mozambik', '258'),
('nepal', 'NP', 'Nepal', 'Nepal', '977'),
('belgium', 'BE', 'Belgium', 'Belçika', '32'),
('bulgaria', 'BG', 'Bulgaria', 'Bulgaristan', '359'),
('hungary', 'HU', 'Hungary', 'Macaristan', '36'),
('moldova', 'MD', 'Moldova', 'Moldova', '373'),
('italy', 'IT', 'Italy', 'İtalya', '39'),
('paraguay', 'PY', 'Paraguay', 'Paraguay', '595'),
('honduras', 'HN', 'Honduras', 'Honduras', '504'),
('tunisia', 'TN', 'Tunisia', 'Tunus', '216'),
('nicaragua', 'NI', 'Nicaragua', 'Nikaragua', '505'),
('timor-leste', 'TL', 'Timor-Leste', 'Doğu Timor', '670'),
('bolivia', 'BO', 'Bolivia', 'Bolivya', '591'),
('costa rica', 'CR', 'Costa Rica', 'Kosta Rika', '506'),
('guatemala', 'GT', 'Guatemala', 'Guatemala', '502'),
('uae', 'AE', 'UAE', 'Birleşik Arap Emirlikleri', '971'),
('zimbabwe', 'ZW', 'Zimbabwe', 'Zimbabve', '263'),
('puerto rico', 'PR', 'Puerto Rico', 'Porto Riko', '1'),
('sudan', 'SD', 'Sudan', 'Sudan', '249'),
('togo', 'TG', 'Togo', 'Togo', '228'),
('kuwait', 'KW', 'Kuwait', 'Kuveyt', '965'),
('salvador', 'SV', 'Salvador', 'El Salvador', '503'),
('libya', 'LY', 'Libya', 'Libya', '218'),
('jamaica', 'JM', 'Jamaica', 'Jamaika', '1'),
('trinidad and tobago', 'TT', 'Trinidad and Tobago', 'Trinidad ve Tobago', '1'),
('ecuador', 'EC', 'Ecuador', 'Ekvador', '593'),
('swaziland', 'SZ', 'Swaziland', 'Esvatini', '268'),
('oman', 'OM', 'Oman', 'Umman', '968'),
('bosnia', 'BA', 'Bosnia', 'Bosna-Hersek', '387'),
('dominican republic', 'DO', 'Dominican Republic', 'Dominik Cumhuriyeti', '1'),
('syria', 'SY', 'Syria', 'Suriye', '963'),
('qatar', 'QA', 'Qatar', 'Katar', '974'),
('panama', 'PA', 'Panama', 'Panama', '507'),
('cuba', 'CU', 'Cuba', 'Küba', '53'),
('mauritania', 'MR', 'Mauritania', 'Moritanya', '222'),
('sierra leone', 'SL', 'Sierra Leone', 'Sierra Leone', '232'),
('jordan', 'JO', 'Jordan', 'Ürdün', '962'),
('portugal', 'PT', 'Portugal', 'Portekiz', '351'),
('barbados', 'BB', 'Barbados', 'Barbados', '1'),
('burundi', 'BI', 'Burundi', 'Burundi', '257'),
('benin', 'BJ', 'Benin', 'Benin', '229'),
('brunei', 'BN', 'Brunei', 'Brunei', '673'),
('bahamas', 'BS', 'Bahamas', 'Bahamalar', '1'),
('botswana', 'BW', 'Botswana', 'Botsvana', '267'),
('belize', 'BZ', 'Belize', 'Belize', '501'),
('central african republic', 'CF', 'Central African Republic', 'Orta Afrika Cumhuriyeti', '236'),
('dominica', 'DM', 'Dominica', 'Dominika', '1'),
('grenada', 'GD', 'Grenada', 'Grenada', '1'),
('georgia', 'GE', 'Georgia', 'Gürcistan', '995'),
('greece', 'GR', 'Greece', 'Yunanistan', '30'),
('guinea-bissau', 'GW', 'Guinea-Bissau', 'Gine-Bissau', '245'),
('guyana', 'GY', 'Guyana', 'Guyana', '592'),
('iceland', 'IS', 'Iceland', 'İzlanda', '354'),
('comoros', 'KM', 'Comoros', 'Komorlar', '269'),
('saint kitts and nevis', 'KN', 'Saint Kitts and Nevis', 'Saint Kitts ve Nevis', '1'),
('liberia', 'LR', 'Liberia', 'Liberya', '231'),
('lesotho', 'LS', 'Lesotho', 'Lesotho', '266'),
('malawi', 'MW', 'Malawi', 'Malavi', '265'),
('namibia', 'NA', 'Namibia', 'Namibya', '264'),
('niger', 'NE', 'Niger', 'Nijer', '227'),
('rwanda', 'RW', 'Rwanda', 'Ruanda', '250'),
('slovakia', 'SK', 'Slovakia', 'Slovakya', '421'),
('suriname', 'SR', 'Suriname', 'Surinam', '597'),
('tajikistan', 'TJ', 'Tajikistan', 'Tacikistan', '992'),
('monaco', 'MC', 'Monaco', 'Monako', '377'),
('bahrain', 'BH', 'Bahrain', 'Bahreyn', '973'),
('reunion', 'RE', 'Reunion', 'Réunion', '262'),
('zambia', 'ZM', 'Zambia', 'Zambiya', '260'),
('armenia', 'AM', 'Armenia', 'Ermenistan', '374'),
('somalia', 'SO', 'Somalia', 'Somali', '252'),
('congo', 'CG', 'Congo', 'Kongo', '242'),
('chile', 'CL', 'Chile', 'Şili', '56'),
('burkina faso', 'BF', 'Burkina Faso', 'Burkina Faso', '226'),
('lebanon', 'LB', 'Lebanon', 'Lübnan', '961'),
('gabon', 'GA', 'Gabon', 'Gabon', '241'),
('albania', 'AL', 'Albania', 'Arnavutluk', '355'),
('uruguay', 'UY', 'Uruguay', 'Uruguay', '598'),
('mauritius', 'MU', 'Mauritius', 'Mauritius', '230'),
('bhutan', 'BT', 'Bhutan', 'Butan', '975'),
('maldives', 'MV', 'Maldives', 'Maldivler', '960'),
('guadeloupe', 'GP', 'Guadeloupe', 'Guadeloupe', '590'),
('turkmenistan', 'TM', 'Turkmenistan', 'Türkmenistan', '993'),
('french guiana', 'GF', 'French Guiana', 'Fransız Guyanası', '594'),
('finland', 'FI', 'Finland', 'Finlandiya', '358'),
('saint lucia', 'LC', 'Saint Lucia', 'Saint Lucia', '1'),
('luxembourg', 'LU', 'Luxembourg', 'Lüksemburg', '352'),
('saint vincent and the grenadines', 'VC', 'Saint Vincent and the Grenadines', 'Saint Vincent ve Grenadinler', '1'),
('equatorial guinea', 'GQ', 'Equatorial Guinea', 'Ekvator Ginesi', '240'),
('djibouti', 'DJ', 'Djibouti', 'Cibuti', '253'),
('antigua and barbuda', 'AG', 'Antigua and Barbuda', 'Antigua ve Barbuda', '1'),
('cayman islands', 'KY', 'Cayman Islands', 'Cayman Adaları', '1'),
('montenegro', 'ME', 'Montenegro', 'Karadağ', '382'),
('denmark', 'DK', 'Denmark', 'Danimarka', '45'),
('switzerland', 'CH', 'Switzerland', 'İsviçre', '41'),
('norway', 'NO', 'Norway', 'Norveç', '47'),
('australia', 'AU', 'Australia', 'Avustralya', '61'),
('eritrea', 'ER', 'Eritrea', 'Eritre', '291'),
('south sudan', 'SS', 'South Sudan', 'Güney Sudan', '211'),
('sao tome and principe', 'ST', 'Sao Tome and Principe', 'São Tomé ve Príncipe', '239'),
('aruba', 'AW', 'Aruba', 'Aruba', '297'),
('montserrat', 'MS', 'Montserrat', 'Montserrat', '1'),
('anguilla', 'AI', 'Anguilla', 'Anguilla', '1'),
('japan', 'JP', 'Japan', 'Japonya', '81'),
('north macedonia', 'MK', 'North Macedonia', 'Kuzey Makedonya', '389'),
('seychelles', 'SC', 'Seychelles', 'Seyşeller', '248'),
('new caledonia', 'NC', 'New Caledonia', 'Yeni Kaledonya', '687'),
('cape verde', 'CV', 'Cape Verde', 'Cabo Verde', '238'),
('usa', 'US', 'USA', 'Amerika Birleşik Devletleri', '1'),
('palestine', 'PS', 'Palestine', 'Filistin', '970'),
('fiji', 'FJ', 'Fiji', 'Fiji', '679'),
('singapore', 'SG', 'Singapore', 'Singapur', '65'),
('samoa', 'WS', 'Samoa', 'Samoa', '685'),
('malta', 'MT', 'Malta', 'Malta', '356'),
('liechtenstein', 'LI', 'Liechtenstein', 'Lihtenştayn', '423'),
('gibraltar', 'GI', 'Gibraltar', 'Cebelitarık', '350'),
('kosovo', 'XK', 'Kosovo', 'Kosova', '383'),
('niue', 'NU', 'Niue', 'Niue', '683'),
('south korea', 'KR', 'South Korea', 'Güney Kore', '82');

-- Takma adlar: aynı ülkenin farklı yazımları. Kanonik satırdan ISO2, Türkçe ad
-- ve telefon kodu devralınır — tekrar yazılmaz, çünkü iki yerde tutulan veri
-- er geç ayrışır.
INSERT INTO country_reference (name_key, iso2, name_en, name_tr, phone_code)
SELECT a.name_key, a.iso2, r.name_en, r.name_tr, r.phone_code
FROM (VALUES
('united states', 'US'),
('united states of america', 'US'),
('russian federation', 'RU'),
('czech republic', 'CZ'),
('czechia', 'CZ'),
('united arab emirates', 'AE'),
('el salvador', 'SV'),
('papua new guinea', 'PG'),
('cote d''ivoire', 'CI'),
('côte d''ivoire', 'CI'),
('bosnia and herzegovina', 'BA'),
('eswatini', 'SZ'),
('congo (drc)', 'CD'),
('democratic republic of the congo', 'CD'),
('republic of the congo', 'CG'),
('macedonia', 'MK'),
('cabo verde', 'CV'),
('east timor', 'TL'),
('great britain', 'GB'),
('uk', 'GB'),
('england', 'GB'),
('south korea', 'KR'),
('korea', 'KR'),
('vietnam (viet nam)', 'VN')
) AS a(name_key, iso2)
JOIN country_reference r ON r.iso2 = a.iso2
WHERE NOT EXISTS (SELECT 1 FROM country_reference x WHERE x.name_key = a.name_key);

-- ── GERİYE DÖNÜK DÜZELTME ─────────────────────────────────────────────
--
-- Halihazırda sağlayıcı kodu ile açılmış ülkeler var. Bunları ISO2'ye taşırız.
-- İki durum var ve İKİSİ DE ele alınmalıdır:
--   (a) Hedef ISO2 henüz yok  → satırı güncelle
--   (b) Hedef ISO2 zaten var  → BİRLEŞTİR: bağımlıları taşı, kopyayı sil
-- (b) atlanırsa UNIQUE(iso2) ihlali migration'ı düşürür.

-- (b) Birleştirme: bağımlı kayıtları kanonik satıra taşı.
WITH bozuk AS (
    SELECT c.id AS dup_id, r.iso2
    FROM countries c
    JOIN country_reference r ON r.name_key = lower(c.name)
    WHERE c.iso2 !~ '^[A-Za-z]{2}$'
), esleme AS (
    SELECT b.dup_id, k.id AS kanonik_id
    FROM bozuk b
    JOIN countries k ON k.iso2 = b.iso2
    WHERE k.id <> b.dup_id
)
UPDATE provider_dimension_maps m
SET local_id = e.kanonik_id
FROM esleme e
WHERE m.dimension = 'country' AND m.local_id = e.dup_id
  AND NOT EXISTS (
    SELECT 1 FROM provider_dimension_maps x
    WHERE x.provider_id = m.provider_id AND x.dimension = 'country'
      AND x.remote_code = m.remote_code AND x.local_id = e.kanonik_id
  );

-- Kopya ülkeye bağlı ürünleri kanonik ülkeye taşı; aynı (servis,ülke,...)
-- ürün zaten varsa kopyayı bırak (aşağıda silinecek).
WITH bozuk AS (
    SELECT c.id AS dup_id, r.iso2
    FROM countries c
    JOIN country_reference r ON r.name_key = lower(c.name)
    WHERE c.iso2 !~ '^[A-Za-z]{2}$'
), esleme AS (
    SELECT b.dup_id, k.id AS kanonik_id
    FROM bozuk b JOIN countries k ON k.iso2 = b.iso2 WHERE k.id <> b.dup_id
)
UPDATE products p
SET country_id = e.kanonik_id
FROM esleme e
WHERE p.country_id = e.dup_id
  AND NOT EXISTS (
    SELECT 1 FROM products x
    -- Benzersizlik anahtarının TAMAMI (products_unique_sku). Eksik bir alan
    -- bırakmak, taşınabilir bir ürünü taşınamaz sanıp silmemize yol açardı.
    WHERE x.kind = p.kind
      AND x.service_id IS NOT DISTINCT FROM p.service_id
      AND x.country_id = e.kanonik_id
      AND x.operator_id IS NOT DISTINCT FROM p.operator_id
      AND x.verification_type = p.verification_type
      AND x.duration_minutes IS NOT DISTINCT FROM p.duration_minutes
      AND x.dimension_a_id IS NOT DISTINCT FROM p.dimension_a_id
  );

-- Taşınamayan (kanonikte karşılığı zaten olan) kopya ürünleri sil.
DELETE FROM products p
USING countries c, country_reference r
WHERE p.country_id = c.id
  AND r.name_key = lower(c.name)
  AND c.iso2 !~ '^[A-Za-z]{2}$'
  AND EXISTS (SELECT 1 FROM countries k WHERE k.iso2 = r.iso2 AND k.id <> c.id);

-- Artık boşalan kopya ülkeleri sil.
DELETE FROM countries c
USING country_reference r
WHERE r.name_key = lower(c.name)
  AND c.iso2 !~ '^[A-Za-z]{2}$'
  AND EXISTS (SELECT 1 FROM countries k WHERE k.iso2 = r.iso2 AND k.id <> c.id);

-- (a) Kalanları yerinde düzelt.
UPDATE countries c
SET iso2 = r.iso2,
    name_tr = CASE WHEN c.name_tr = '' THEN r.name_tr ELSE c.name_tr END,
    phone_code = CASE WHEN c.phone_code = '' THEN r.phone_code ELSE c.phone_code END,
    updated_at = now()
FROM country_reference r
WHERE r.name_key = lower(c.name)
  AND c.iso2 !~ '^[A-Za-z]{2}$';

-- Eşleşmeyen ülkeler GİZLENİR, silinmez.
--
-- Silmek, sağlayıcı bir ülkeyi yeni bir adla döndürdüğünde o ülkeye ait tüm
-- ürün geçmişini yok ederdi. Gizlemek katalogdan çıkarır ama veriyi korur ve
-- referansa takma ad eklendiğinde kendiliğinden geri gelir.
UPDATE countries SET is_visible = false, updated_at = now()
WHERE iso2 !~ '^[A-Za-z]{2}$' AND is_visible;

-- Mevcut Türkçe adı ve telefon kodu boş olanları referanstan tamamla.
UPDATE countries c
SET name_tr = CASE WHEN c.name_tr = '' THEN r.name_tr ELSE c.name_tr END,
    phone_code = CASE WHEN c.phone_code = '' THEN r.phone_code ELSE c.phone_code END,
    updated_at = now()
FROM country_reference r
WHERE r.iso2 = c.iso2 AND (c.name_tr = '' OR c.phone_code = '');

-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin
DROP TABLE IF EXISTS country_reference;
-- +goose StatementEnd
