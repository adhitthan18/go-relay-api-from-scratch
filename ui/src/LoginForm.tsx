import { useState } from "react";
import { api, type Me } from "./api";

export function LoginForm({ onMasuk }: { onMasuk: (me: Me) => void }) {
  const [email, setEmail] = useState("");
  const [password, setPassword] = useState("");
  const [galat, setGalat] = useState("");
  const [mengirim, setMengirim] = useState(false);

  async function kirim(e: React.FormEvent) {
    e.preventDefault();
    setGalat("");
    setMengirim(true);

    try {
      onMasuk(await api.login(email, password));
    } catch (err) {
      setGalat(err instanceof Error ? err.message : "gagal masuk");
    } finally {
      setMengirim(false);
      setPassword("");
    }
  }

  return (
    <section>
      <h1>Masuk</h1>
      <form onSubmit={kirim}>
        <label htmlFor="email">Email</label>
        <input
          id="email"
          type="email"
          autoComplete="username"
          required
          value={email}
          onChange={(e) => setEmail(e.target.value)}
        />

        <label htmlFor="password">Password</label>
        <input
          id="password"
          type="password"
          autoComplete="current-password"
          required
          value={password}
          onChange={(e) => setPassword(e.target.value)}
        />

        <button type="submit" disabled={mengirim}>
          {mengirim ? "Memeriksa…" : "Masuk"}
        </button>

        <p className="galat" role="alert">{galat}</p>
      </form>
    </section>
  );
}