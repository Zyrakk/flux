import typography from '@tailwindcss/typography';

/** @type {import('tailwindcss').Config} */
export default {
	content: ['./src/**/*.{html,js,svelte,ts}'],
	darkMode: 'class',
	theme: {
		extend: {
			fontFamily: {
				sans: ['Plus Jakarta Sans', 'ui-sans-serif', 'system-ui', 'sans-serif'],
				mono: ['JetBrains Mono', 'ui-monospace', 'SFMono-Regular', 'monospace']
			},
			colors: {
				flux: {
					bg:     '#06080c',
					soft:   '#0c1018',
					panel:  '#0f1420',
					accent: '#06b6d4',
				}
			},
			borderRadius: {
				'2xl': '16px',
				'3xl': '24px',
			},
			backdropBlur: {
				'glass': '20px',
			}
		}
	},
	plugins: [typography]
};
