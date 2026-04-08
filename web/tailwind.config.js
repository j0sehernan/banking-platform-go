/** @type {import('tailwindcss').Config} */
module.exports = {
  content: [
    './app/**/*.{js,ts,jsx,tsx,mdx}',
    './components/**/*.{js,ts,jsx,tsx,mdx}',
  ],
  theme: {
    extend: {
      colors: {
        ink: {
          950: '#0a0e1a',
          900: '#0f1420',
          800: '#1a2030',
          700: '#252e44',
        },
      },
    },
  },
  plugins: [],
};
