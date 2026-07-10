import type { Config } from "tailwindcss";

const config: Config = {
  content: ["./src/**/*.{ts,tsx}"],
  theme: {
    extend: {
      fontFamily: {
        sans: ["IBM Plex Sans Thai", "ui-sans-serif", "system-ui", "sans-serif"]
      },
      colors: {
        surface: {
          0: "#ffffff",
          50: "#f7f7f5",
          100: "#ecece7",
          200: "#dadad2",
          900: "#121212"
        }
      },
      boxShadow: {
        panel: "0 16px 60px rgba(0, 0, 0, 0.08)"
      }
    }
  },
  plugins: []
};

export default config;
