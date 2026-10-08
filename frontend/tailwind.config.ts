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
        destructive: {
          DEFAULT: "hsl(var(--destructive))",
          foreground: "hsl(var(--destructive-foreground))"
        },
        ring: "hsl(var(--ring))",

        // Neutral scale — Material-grey based. Literal, so it does not follow
        // the theme: for surfaces use bg-muted / bg-card, not neutral-*.
        neutral: {
          DEFAULT: "#9E9E9E",
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
        // The tint stops (50/100/200) and the text stops (700/800) are CSS
        // variables with a dark-theme value in globals.css, so a status tint
        // follows the theme by itself — opacity and hover variants included.
        // Solid fills (500/600, DEFAULT) stay literal; buttons darken them with
        // opacity rather than by stepping to 700.
        success: {
          DEFAULT: "#16A34A",
          foreground: "#FFFFFF",
          50: "rgb(var(--success-50) / <alpha-value>)",
          100: "rgb(var(--success-100) / <alpha-value>)",
          200: "rgb(var(--success-200) / <alpha-value>)",
          300: "#86EFAC",
          400: "#4ADE80",
          500: "#22C55E",
          600: "#16A34A",
          700: "rgb(var(--success-700) / <alpha-value>)",
          800: "rgb(var(--success-800) / <alpha-value>)",
          900: "#14532D"
        },
        error: {
          DEFAULT: "rgb(var(--error) / <alpha-value>)",
          foreground: "#FFFFFF",
          50: "rgb(var(--error-50) / <alpha-value>)",
          100: "rgb(var(--error-100) / <alpha-value>)",
          200: "rgb(var(--error-200) / <alpha-value>)",
          300: "#FCA5A5",
          400: "#F87171",
          500: "#EF4444",
          600: "#DC2626",
          700: "rgb(var(--error-700) / <alpha-value>)",
          800: "rgb(var(--error-800) / <alpha-value>)",
          900: "#7F1D1D"
        },
        warning: {
          DEFAULT: "#D97706",
          foreground: "#212121",
          50: "rgb(var(--warning-50) / <alpha-value>)",
          100: "rgb(var(--warning-100) / <alpha-value>)",
          200: "rgb(var(--warning-200) / <alpha-value>)",
          300: "#FCD34D",
          400: "#FBBF24",
          500: "#F59E0B",
          600: "#D97706",
          700: "rgb(var(--warning-700) / <alpha-value>)",
          800: "rgb(var(--warning-800) / <alpha-value>)",
          900: "#78350F"
        },
        info: {
          DEFAULT: "#0284C7",
          foreground: "#FFFFFF",
          50: "rgb(var(--info-50) / <alpha-value>)",
          100: "rgb(var(--info-100) / <alpha-value>)",
          200: "rgb(var(--info-200) / <alpha-value>)",
          300: "#7DD3FC",
          400: "#38BDF8",
          500: "#0EA5E9",
          600: "#0284C7",
          700: "rgb(var(--info-700) / <alpha-value>)",
          800: "rgb(var(--info-800) / <alpha-value>)",
          900: "#0C4A6E"
        },

        // Surfaces with their own dark value (globals.css), usable with
        // opacity and hover variants like any other color.
        "surface-warm": "hsl(var(--surface-warm) / <alpha-value>)",
        "pos-canvas": "hsl(var(--pos-canvas) / <alpha-value>)",
        placeholder: {
          from: "hsl(var(--placeholder-from) / <alpha-value>)",
          to: "hsl(var(--placeholder-to) / <alpha-value>)"
        }
      },
      // One ordered scale: sm (6) < md (10, controls) < lg = xl (12) < 2xl
      // (16, cards and panels) < 3xl (24). --radius used to be 1rem, which put
      // rounded-lg above rounded-xl and turned a rounded-sm checkbox round.
      borderRadius: {
        lg: "var(--radius)",
        md: "calc(var(--radius) - 2px)",
        sm: "calc(var(--radius) - 6px)"
      },
      boxShadow: {
        card: "var(--shadow-card)",
        brand: "var(--shadow-brand)"
      },
      fontSize: {
        // The one step below text-xs, for count badges and chart axis labels.
        "2xs": ["0.6875rem", { lineHeight: "1rem" }]
      },
      spacing: {
        sidebar: "232px",
        "sidebar-collapsed": "72px"
      }
    }
  },
  plugins: []
};

export default config;
