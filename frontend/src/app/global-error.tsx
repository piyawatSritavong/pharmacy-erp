"use client";

/**
 * Last resort, when the root layout itself fails. It replaces the whole
 * document, so it brings its own <html> and plain styles.
 */
export default function GlobalError({ reset }: { error: Error & { digest?: string }; reset: () => void }) {
  return (
    <html lang="th">
      <body style={{ alignItems: "center", display: "grid", fontFamily: "system-ui, sans-serif", margin: 0, minHeight: "100vh", padding: 24, placeItems: "center" }}>
        <main style={{ maxWidth: 420, textAlign: "center" }}>
          <h1 style={{ fontSize: 20, marginBottom: 8 }}>ระบบขัดข้องชั่วคราว</h1>
          <p style={{ color: "#555", fontSize: 14, lineHeight: 1.6 }}>กรุณาลองใหม่อีกครั้ง หากยังไม่ได้ให้แจ้งผู้ดูแลระบบ</p>
          <button onClick={reset} style={{ background: "#344ABF", border: 0, borderRadius: 10, color: "#fff", cursor: "pointer", fontSize: 14, marginTop: 16, padding: "10px 20px" }} type="button">
            ลองใหม่
          </button>
        </main>
      </body>
    </html>
  );
}
