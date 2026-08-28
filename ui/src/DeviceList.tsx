import { useEffect, useRef, useState } from "react";
import { api, type Device } from "./api";
import { ClaimForm } from "./ClaimForm";

const INTERVAL_SEGAR = 3000;

export function DeviceList() {
  const [perangkat, setPerangkat] = useState<Device[] | null>(null);
  const [status, setStatus] = useState("");
  const [galat, setGalat] = useState("");
  const [pemicu, setPemicu] = useState(0);

  // Ref, bukan state: dipakai di dalam useEffect tanpa ikut jadi dependensi.
  const punyaData = useRef(false);

  useEffect(() => {
    let dibatalkan = false;

    async function muat() {
      try {
        const data = await api.devices();
        if (dibatalkan) return;
        setPerangkat(data);
        punyaData.current = true;
        setGalat("");
      } catch (err) {
        // Kegagalan satu putaran tidak boleh mengosongkan layar. Galat hanya
        // ditampilkan kalau memang belum pernah ada data sama sekali.
        if (dibatalkan || punyaData.current) return;
        setGalat(err instanceof Error ? err.message : "gagal memuat");
      }
    }

    muat();
    const timer = setInterval(muat, INTERVAL_SEGAR);

    return () => {
      dibatalkan = true;
      clearInterval(timer);
    };
  }, [pemicu]);

  const segarkan = () => setPemicu((n) => n + 1);

  async function kirim(d: Device, state: "ON" | "OFF") {
    setStatus(`Mengirim ${state}…`);
    try {
      await api.kirimPerintah(d.id, state);
      setStatus(`Perintah ${state} dikirim ke ${d.display_name}.`);
    } catch (err) {
      setStatus(err instanceof Error ? `Gagal: ${err.message}` : "Gagal");
    }
  }

  async function lepas(d: Device) {
    // Melepas perangkat yang salah berarti seseorang harus datang ke lokasi
    // untuk memasangkannya lagi. Tindakan semahal itu pantas dikonfirmasi.
    if (!confirm(`Lepas "${d.display_name}"? Perangkat akan menampilkan QR baru.`)) {
      return;
    }
    try {
      await api.release(d.id);
      setStatus(`${d.display_name} dilepas.`);
      segarkan();
    } catch (err) {
      setStatus(err instanceof Error ? `Gagal: ${err.message}` : "Gagal");
    }
  }

  if (galat) return <p className="galat">{galat}</p>;
  if (!perangkat) return <p className="samar">Memuat perangkat…</p>;

  return (
    <>
      <ClaimForm onSelesai={segarkan} />

      {perangkat.length === 0 && <p className="samar">Belum ada perangkat terdaftar.</p>}

      {perangkat.map((d) => {
        const aktif = d.status === "active";

        return (
          <div key={d.id} className={`perangkat ${aktif ? "" : "menunggu"}`}>
            <div className="baris-judul">
              <h2>{d.display_name}</h2>
              {aktif ? (
                <span className={`lampu ${d.online ? "hidup" : "putus"}`}>
                  {d.online ? "Online" : "Offline"}
                </span>
              ) : (
                <span className="lampu menunggu">Menunggu diklaim</span>
              )}
            </div>

            <p className="samar"><code>{d.hardware_id}</code></p>

            {aktif ? (
              <>
                <p className="keadaan">
                  Relay:{" "}
                  {d.last_known_state === null ? (
                    <span className="samar">belum diketahui</span>
                  ) : (
                    <strong className={d.last_known_state === "ON" ? "on" : "off"}>
                      {d.last_known_state === "ON" ? "MENYALA" : "MATI"}
                    </strong>
                  )}
                </p>

                <div className="tombol">
                  <button onClick={() => kirim(d, "ON")}>Nyalakan</button>
                  <button className="mati" onClick={() => kirim(d, "OFF")}>Matikan</button>
                  <button className="sekunder" onClick={() => lepas(d)}>Lepas</button>
                </div>
              </>
            ) : (
              <p className="samar">
                Perangkat ini sedang menampilkan QR. Pindai QR-nya di formulir di atas.
              </p>
            )}
          </div>
        );
      })}

      <p className="samar" role="status">{status}</p>
    </>
  );
}