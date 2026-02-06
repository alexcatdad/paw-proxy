/** @type {import('tailwindcss').Config} */
module.exports = {
    content: ['./docs/index.html', './docs/404.html'],
    darkMode: 'class',
    theme: {
        extend: {
            fontFamily: {
                sans: ['Nunito', 'system-ui', 'sans-serif'],
                mono: ['JetBrains Mono', 'monospace'],
            },
            colors: {
                sand: {
                    50: '#FDFBF7',
                    100: '#FAF6EF',
                    200: '#F3EBE0',
                    300: '#E8DCC8',
                    400: '#D4C4A8',
                    500: '#B8A584',
                    600: '#8C7A5E',
                    700: '#5C5142',
                    800: '#3D362D',
                    900: '#252118',
                },
                slate: {
                    750: '#1E2433',
                    850: '#141926',
                    925: '#0D1117',
                    950: '#080B10',
                },
                accent: {
                    light: '#FF8A4C',
                    DEFAULT: '#F97316',
                    dark: '#EA580C',
                },
                mint: {
                    light: '#6EE7B7',
                    DEFAULT: '#34D399',
                    dark: '#10B981',
                }
            },
            borderRadius: {
                '4xl': '2rem',
                '5xl': '2.5rem',
            }
        }
    },
    plugins: [],
}
