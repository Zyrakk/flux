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

function normalizeSectionName(sectionName?: string): string {
	const name = (sectionName ?? '').toLowerCase();
	if (name.includes('cyber')) return 'cybersecurity';
	if (name.includes('tech')) return 'tech';
	if (name.includes('econ')) return 'economy';
	if (name.includes('world') || name.includes('geo')) return 'world';
	return name;
}

export function sectionColor(sectionName?: string): string {
	switch (normalizeSectionName(sectionName)) {
		case 'cybersecurity':
			return '#06b6d4';
		case 'tech':
			return '#a78bfa';
		case 'economy':
			return '#fbbf24';
		case 'world':
			return '#34d399';
		default:
			return '#06b6d4';
	}
}

export function sectionTint(sectionName?: string): string {
	switch (normalizeSectionName(sectionName)) {
		case 'cybersecurity':
			return '6,182,212';
		case 'tech':
			return '167,139,250';
		case 'economy':
			return '251,191,36';
		case 'world':
			return '52,211,153';
		default:
			return '6,182,212';
	}
}

export function priorityColor(priority?: string): string {
	switch ((priority ?? '').toUpperCase()) {
		case 'CRITICAL':
			return '#ef4444';
		case 'HIGH':
			return '#fbbf24';
		case 'MEDIUM':
			return '#06b6d4';
		case 'LOW':
			return '#94a3b8';
		default:
			return '#06b6d4';
	}
}

export function priorityLabel(score?: number): string {
	if (score == null || Number.isNaN(score)) return 'MEDIUM';
	if (score >= 0.85) return 'CRITICAL';
	if (score >= 0.65) return 'HIGH';
	if (score >= 0.35) return 'MEDIUM';
	return 'LOW';
}

export function sourceBadge(sourceType: string): { icon: string; label: string; className: string } {
	switch (sourceType?.toLowerCase()) {
		case 'hn':
			return { icon: '◧', label: 'HN', className: 'source-badge source-badge--hn' };
		case 'reddit':
			return { icon: '◉', label: 'Reddit', className: 'source-badge source-badge--reddit' };
		case 'github':
			return { icon: '◈', label: 'GitHub', className: 'source-badge source-badge--github' };
		case 'rss':
		default:
			return { icon: '◆', label: 'RSS', className: 'source-badge source-badge--rss' };
	}
}

export function isSameCalendarDay(a: string, b: Date = new Date()): boolean {
	const d = new Date(a);
	if (Number.isNaN(d.getTime())) {
		return false;
	}
	return d.getFullYear() === b.getFullYear() && d.getMonth() === b.getMonth() && d.getDate() === b.getDate();
}
