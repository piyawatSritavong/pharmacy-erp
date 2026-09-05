import type { Config } from "tailwindcss";

const config: Config = {
  // Toggled, not system-sniffed: the operator's choice is stored and applied
  // before paint (see ThemeScript), so a shop floor screen keeps the setting it
  // was left on.
  darkMode: "class",
  content: ["./src/**/*.{ts,tsx}"],
  theme: {
    extend: {
      fontFamily: {
        sans: ["var(--font-sans)", "Noto Sans Thai", "ui-sans-serif", "system-ui", "sans-serif"]
      },
      colors: {
        background: "hsl(var(--background))",
        foreground: "hsl(var(--foreground))",
        card: {
          DEFAULT: "hsl(var(--card))",
          foreground: "hsl(var(--card-foreground))"
        },
        muted: {
          DEFAULT: "hsl(var(--muted))",
          foreground: "hsl(var(--muted-foreground))"
        },
        border: "hsl(var(--border))",
        input: "hsl(var(--input))",

        // Brand primary — #344ABF. DEFAULT/foreground stay themeable via CSS
        // vars (see globals.css); the 50-900 scale is literal so hover/active
        // states (a shade darker) and tints work without editing globals.css.
        primary: {
          DEFAULT: "hsl(var(--primary))",
          foreground: "hsl(var(--primary-foreground))",
          50: "#E6E8F8",
          100: "#C0C7EE",
          200: "#96A1E3",
          300: "#6D7CD7",
          400: "#4D60CF",
          500: "#344ABF", // brand primary
          600: "#3143B3", // hover
          700: "#2C3DA3", // active
          800: "#283793",
          900: "#1D286A"
        },
        secondary: {
          DEFAULT: "hsl(var(--secondary))",
          foreground: "hsl(var(--secondary-foreground))"
        },
        accent: {
          DEFAULT: "hsl(var(--accent))",
          foreground: "hsl(var(--accent-foreground))"
        },
        destructive: {
          DEFAULT: "hsl(var(--destructive))",
          foreground: "hsl(var(--destructive-foreground))"
        },
        ring: "hsl(var(--ring))",

        // Neutral scale — Material-grey based so the two named brand values
        // land on exact stops: neutral-light = neutral-500 (#9E9E9E),
        // neutral-dark = neutral-900 (#212121). The `light`/`dark`/`white`
        // aliases make `bg-neutral-light` / `bg-neutral-dark` work as literal
        // utility names per the design spec, alongside the numeric scale for
        // anything in between.
        neutral: {
          DEFAULT: "#9E9E9E",
          light: "#9E9E9E",
          dark: "#212121",
          white: "#FFFFFF",
          50: "#FAFAFA",
          100: "#F5F5F5",
          200: "#EEEEEE",
          300: "#E0E0E0",
          400: "#BDBDBD",
          500: "#9E9E9E",
          600: "#757575",
          700: "#616161",
          800: "#424242",
          900: "#212121"
        },

        // Semantic colors, each with a 50-900 scale + DEFAULT/foreground so
        // status badges, banners, and buttons all draw from the same tokens.
        success: {
          DEFAULT: "#16A34A",
          foreground: "#FFFFFF",
          50: "#F0FDF4",
          100: "#DCFCE7",
          200: "#BBF7D0",
          300: "#86EFAC",
          400: "#4ADE80",
          500: "#22C55E",
          600: "#16A34A",
          700: "#15803D",
          800: "#166534",
          900: "#14532D"
        },
        error: {
          DEFAULT: "#DC2626",
          foreground: "#FFFFFF",
          50: "#FEF2F2",
          100: "#FEE2E2",
          200: "#FECACA",
          300: "#FCA5A5",
          400: "#F87171",
          500: "#EF4444",
          600: "#DC2626",
          700: "#B91C1C",
          800: "#991B1B",
          900: "#7F1D1D"
        },
        warning: {
          DEFAULT: "#D97706",
          foreground: "#212121",
          50: "#FFFBEB",
          100: "#FEF3C7",
          200: "#FDE68A",
          300: "#FCD34D",
          400: "#FBBF24",
          500: "#F59E0B",
          600: "#D97706",
          700: "#B45309",
          800: "#92400E",
          900: "#78350F"
        },
        info: {
          DEFAULT: "#0284C7",
          foreground: "#FFFFFF",
          50: "#F0F9FF",
          100: "#E0F2FE",
          200: "#BAE6FD",
          300: "#7DD3FC",
          400: "#38BDF8",
          500: "#0EA5E9",
          600: "#0284C7",
          700: "#0369A1",
          800: "#075985",
          900: "#0C4A6E"
        },

        // Legacy scale kept for compatibility with older section styles.
        surface: {
          0: "#ffffff",
          50: "#f4f4f5",
          100: "#e4e4e7",
          200: "#d4d4d8",
          900: "#18181b"
        }
      },
      borderRadius: {
        lg: "var(--radius)",
        md: "calc(var(--radius) - 2px)",
        sm: "calc(var(--radius) - 4px)"
      },
      boxShadow: {
        panel: "0 1px 2px 0 rgb(0 0 0 / 0.05)"
      }
    }
  },
  plugins: []
};

export default config;
