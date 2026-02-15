import typography from '@tailwindcss/typography';

/** @type {import('tailwindcss').Config} */
export default {
	content: ['./src/**/*.{html,js,svelte,ts}'],
	darkMode: 'class',
	theme: {
		extend: {
			fontFamily: {
				sans: ['Inter', 'ui-sans-serif', 'system-ui', 'sans-serif'],
				mono: ['JetBrains Mono', 'ui-monospace', 'SFMono-Regular', 'monospace']
			},
			colors: {
				bg: {
					0: '#0b0f14',
					1: '#111827',
					2: '#1f2937'
				},
				text: {
					0: '#f3f4f6',
					1: '#d1d5db',
					2: '#9ca3af'
				}
			}
		}
	},
	plugins: [typography]
};
