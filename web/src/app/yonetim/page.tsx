'use client';

import Link from 'next/link';
import { useSession } from '@/hooks/useSession';
import { Card, Badge, Button, Alert } from '@/components/ui';

export default function AdminHome() {
  const { user } = useSession();

  return (
    <div className="flex flex-col gap-5">
      <div>
        <h1 className="text-2xl font-bold tracking-tight md:text-3xl">Genel bakış</h1>
        <p className="mt-1 text-sm text-muted">Yönetim araçları.</p>
      </div>

      <Card>
        <h2 className="text-lg font-semibold">Yetkileriniz</h2>
        <ul className="mt-3 flex flex-wrap gap-2">
          {user?.permissions.map((p) => <li key={p}><Badge tone="neutral">{p}</Badge></li>)}
        </ul>
      </Card>

      <div className="grid gap-3 sm:grid-cols-2 md:gap-4">
        <Card className="flex flex-col justify-between">
          <div>
            <h2 className="font-semibold">Bakiye düzeltme</h2>
            <p className="mt-1.5 text-sm text-muted">
              Bir kullanıcının bakiyesini elle artırın veya azaltın. Her düzeltme
              kayıt defterine değiştirilemez bir satır olarak yazılır.
            </p>
          </div>
          <Link href="/yonetim/bakiye" className="mt-4">
            <Button variant="outline" size="sm" fullWidth className="sm:w-auto">Aç</Button>
          </Link>
        </Card>

        <Card>
          <h2 className="font-semibold">Sipariş ve kullanıcı yönetimi</h2>
          <p className="mt-1.5 text-sm text-muted">
            Sipariş listesi, iade yönetimi ve kullanıcı arama ekranları
            sipariş akışıyla birlikte açılacak.
          </p>
        </Card>
      </div>

      <Alert tone="info">
        Bu panelde bugün yalnız arka uçta gerçekten var olan uçlar gösterilir.
        Çalışmayan bir düğme koymuyoruz — boş bir ekran, çalışıyormuş gibi
        görünen bozuk bir ekrandan iyidir.
      </Alert>
    </div>
  );
}
