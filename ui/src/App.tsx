import { useEffect, useState } from "react";
import { api, type Me } from "./api";
import { LoginForm } from "./LoginForm";
import { DeviceList } from "./DeviceList";

export default function App() {
  const [me, setMe] = useState<Me | null>(null);
  const [memuat, setMemuat] = useState(true);

  // Sumber kebenaran satu-satunya: apakah server masih menerima session kita.
  // Tidak ada status login yang disimpan di localStorage.
  useEffect(() => {
    api.me()
      .then(setMe)
      .catch(() => setMe(null))
      .finally(() => setMemuat(false));
  }, []);

  if (memuat) return <p className="samar">Memuat…</p>;
  if (!me) return <LoginForm onMasuk={setMe} />;

  return (
    <>
      <header>
        <div>
          <h1>Perangkat</h1>
          <p className="samar">{me.email} · {me.role}</p>
        </div>
        <button
          className="sekunder"
          onClick={async () => {
            await api.logout().catch(() => {});
            setMe(null);
          }}
        >
          Keluar
        </button>
      </header>

      <DeviceList />
    </>
  );
}