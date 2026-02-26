import typography from '@tailwindcss/typography';

/** @type {import('tailwindcss').Config} */
export default {
	content: ['./src/**/*.{html,js,svelte,ts}'],
	darkMode: 'class',
	theme: {
		extend: {
			fontFamily: {
				sans: ['Plus Jakarta Sans', 'ui-sans-serif', 'system-ui', 'sans-serif'],
				mono: ['JetBrains Mono', 'ui-monospace', 'SFMono-Regular', 'monospace'],
				serif: ['Georgia', 'Times New Roman', 'serif']
			},
			colors: {
				flux: {
					bg: '#030508',
					'bg-mid': '#060a10',
					'bg-deep': '#080c14',
					surface: 'rgba(255,255,255,0.018)',
					'surface-hover': 'rgba(255,255,255,0.035)',
					border: 'rgba(255,255,255,0.04)',
					'border-hover': 'rgba(255,255,255,0.1)',
					accent: '#06b6d4',
					violet: '#a78bfa',
					amber: '#fbbf24',
					emerald: '#34d399',
					danger: '#ef4444'
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
