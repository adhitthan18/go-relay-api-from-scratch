import { useEffect, useRef, useState } from "react";
import { Html5Qrcode } from "html5-qrcode";
import { api } from "./api";

const ID_KOTAK = "kotak-pemindai";

export function ClaimForm({ onSelesai }: { onSelesai: () => void }) {
  const [payload, setPayload] = useState("");
  const [nama, setNama] = useState("");
  const [galat, setGalat] = useState("");
  const [memindai, setMemindai] = useState(false);
  const pemindai = useRef<Html5Qrcode | null>(null);

  // Kamera WAJIB dimatikan saat komponen dilepas. Tanpa ini lampu kameranya
  // tetap menyala setelah panel ditutup — dan pengguna berhak curiga.
  useEffect(() => {
    return () => {
      pemindai.current?.stop().catch(() => {});
      pemindai.current = null;
    };
  }, []);

  async function mulaiPindai() {
    setGalat("");
    setMemindai(true);
    try {
      const p = new Html5Qrcode(ID_KOTAK);
      pemindai.current = p;
      await p.start(
        { facingMode: "environment" },
        { fps: 10, qrbox: { width: 240, height: 240 } },
        async (teks) => {
          setPayload(teks);
          await p.stop().catch(() => {});
          pemindai.current = null;
          setMemindai(false);
        },
        () => {}, // dipanggil tiap frame tanpa QR — bukan kesalahan, abaikan
      );
    } catch (err) {
      setMemindai(false);
      setGalat(err instanceof Error ? err.message : "kamera tidak bisa dibuka");
    }
  }

  async function kirim(e: React.FormEvent) {
    e.preventDefault();
    setGalat("");
    try {
      await api.claim(payload.trim(), nama.trim());
      setPayload("");
      setNama("");
      onSelesai();
    } catch (err) {
      setGalat(err instanceof Error ? err.message : "gagal memasangkan");
    }
  }

  return (
    <form className="klaim" onSubmit={kirim}>
      <h2>Pasangkan perangkat baru</h2>

      <div id={ID_KOTAK} className="kotak-pindai" hidden={!memindai} />

      {!memindai && (
        <button type="button" className="sekunder" onClick={mulaiPindai}>
          Buka kamera
        </button>
      )}

      <label htmlFor="payload">Isi QR</label>
      <textarea
        id="payload"
        rows={2}
        required
        placeholder="RELAY1...."
        value={payload}
        onChange={(e) => setPayload(e.target.value)}
      />
      <span className="samar">
        Terisi sendiri setelah dipindai. Tempel manual bila izin kamera ditolak.
      </span>

      <label htmlFor="nama">Nama perangkat</label>
      <input
        id="nama"
        maxLength={120}
        placeholder="Relay Pompa 01"
        value={nama}
        onChange={(e) => setNama(e.target.value)}
      />

      <button type="submit">Pasangkan</button>
      <p className="galat">{galat}</p>
    </form>
  );
}