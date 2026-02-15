export type FeedbackAction = 'like' | 'dislike' | 'save';

export interface Section {
	id: string;
	name: string;
	display_name: string;
	enabled: boolean;
	sort_order: number;
	max_briefing_articles: number;
	seed_keywords: string[];
	article_count?: number;
	active_sources?: number;
}

export interface SourceSectionRef {
	id: string;
	name: string;
	display_name: string;
}

export interface Source {
	id: string;
	source_type: string;
	name: string;
	config: Record<string, unknown>;
	enabled: boolean;
	last_fetched_at?: string;
	error_count: number;
	last_error?: string;
	sections: SourceSectionRef[];
	stats: {
		total_ingested: number;
		last_24h: number;
		pass_rate_pct: number;
	};
}

export interface Article {
	id: string;
	source_type: string;
	source_id: string;
	url: string;
	title: string;
	content?: string;
	summary?: string;
	author?: string;
	published_at?: string;
	ingested_at: string;
	processed_at?: string;
	relevance_score?: number;
	categories?: string[];
	status: string;
	metadata?: Record<string, unknown>;
	section?: {
		id: string;
		name: string;
		display_name: string;
	};
	source: {
		type: string;
		id: string;
		name: string;
		ref?: string;
	};
	feedback: {
		likes: number;
		dislikes: number;
		saves: number;
		liked: boolean;
		disliked: boolean;
		saved: boolean;
		like_id?: string;
		dislike_id?: string;
		save_id?: string;
	};
}

export interface Briefing {
	id: string;
	generated_at: string;
	content: string;
	article_ids: string[];
	metadata?: {
		partial?: boolean;
		pending_count?: number;
		sections?: Record<string, { total: number; filtered: number }>;
		[key: string]: unknown;
	};
	articles: Article[];
}

export interface Feedback {
	id: string;
	article_id: string;
	action: FeedbackAction;
	created_at: string;
}
