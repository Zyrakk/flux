export function formatRelativeTime(input?: string): string {
	if (!input) {
		return 'sin fecha';
	}
	const value = new Date(input).getTime();
	if (Number.isNaN(value)) {
		return 'sin fecha';
	}

	const diffMs = value - Date.now();
	const diffSeconds = Math.round(diffMs / 1000);
	const absSeconds = Math.abs(diffSeconds);
	const rtf = new Intl.RelativeTimeFormat('es', { numeric: 'auto' });

	if (absSeconds < 60) {
		return rtf.format(diffSeconds, 'second');
	}
	const diffMinutes = Math.round(diffSeconds / 60);
	if (Math.abs(diffMinutes) < 60) {
		return rtf.format(diffMinutes, 'minute');
	}
	const diffHours = Math.round(diffMinutes / 60);
	if (Math.abs(diffHours) < 24) {
		return rtf.format(diffHours, 'hour');
	}
	const diffDays = Math.round(diffHours / 24);
	return rtf.format(diffDays, 'day');
}

export function formatDateTime(input?: string): string {
	if (!input) {
		return 'N/A';
	}
	const value = new Date(input);
	if (Number.isNaN(value.getTime())) {
		return 'N/A';
	}
	return value.toLocaleString('es-ES', {
		year: 'numeric',
		month: 'short',
		day: '2-digit',
		hour: '2-digit',
		minute: '2-digit'
	});
}

export function sectionColor(sectionName?: string): string {
	switch ((sectionName ?? '').toLowerCase()) {
	case 'cybersecurity':
		return 'bg-red-500/20 text-red-200 border border-red-400/40';
	case 'tech':
		return 'bg-cyan-500/20 text-cyan-200 border border-cyan-400/40';
	case 'economy':
		return 'bg-emerald-500/20 text-emerald-200 border border-emerald-400/40';
	case 'world':
		return 'bg-indigo-500/20 text-indigo-200 border border-indigo-400/40';
	default:
		return 'bg-slate-700/70 text-slate-100 border border-slate-500';
	}
}

export function sourceBadge(sourceType: string): { icon: string; label: string; className: string } {
	switch (sourceType) {
	case 'hn':
		return { icon: '🟧', label: 'HN', className: 'bg-orange-500/20 text-orange-100 border border-orange-400/40' };
	case 'reddit':
		return { icon: '🔵', label: 'Reddit', className: 'bg-blue-500/20 text-blue-100 border border-blue-400/40' };
	case 'rss':
	default:
		return { icon: '🟠', label: 'RSS', className: 'bg-amber-500/20 text-amber-100 border border-amber-400/40' };
	}
}

export function isSameCalendarDay(a: string, b: Date = new Date()): boolean {
	const d = new Date(a);
	if (Number.isNaN(d.getTime())) {
		return false;
	}
	return d.getFullYear() === b.getFullYear() && d.getMonth() === b.getMonth() && d.getDate() === b.getDate();
}
