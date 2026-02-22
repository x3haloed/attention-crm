/** @type {import('tailwindcss').Config} */
module.exports = {
  content: [
    './docs/design/**/*.html',
    './internal/**/*.go',
    './web/**/*.html'
  ],
  theme: {
    fontFamily: {
      sans: [
        'Inter',
        'ui-sans-serif',
        'system-ui',
        '-apple-system',
        'Segoe UI',
        'Roboto',
        'Helvetica Neue',
        'Arial',
        'Noto Sans',
        'Liberation Sans',
        'sans-serif',
        'Apple Color Emoji',
        'Segoe UI Emoji',
        'Segoe UI Symbol',
        'Noto Color Emoji'
      ]
    },
    extend: {}
  },
  plugins: []
}
