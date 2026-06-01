import { HttpClient, HttpErrorResponse, HttpHeaders } from '@angular/common/http';
import { Injectable } from '@angular/core';
import { firstValueFrom } from 'rxjs';
import { RdioScannerAdminService } from '../admin/admin.service';
import { RdioScannerService } from '../rdio-scanner.service';

export interface TranscriptReviewItem {
    callId: number;
    timestamp: number;
    transcript: string;
    reviewedTranscript: string;
    trainingReviewStatus: string;
    transcriptionStatus?: string;
    systemLabel: string;
    talkgroupLabel: string;
    talkgroupName?: string;
}

export interface TranscriptReviewListResponse {
    items: TranscriptReviewItem[];
    offset: number;
    limit: number;
    collectorConfigured: boolean;
    source?: 'admin' | 'alerts' | 'none';
    adminError?: string;
    alertsError?: string;
    hasPin?: boolean;
}

export interface CollectorContributionStats {
    serverName?: string;
    serverUrl?: string;
    submissions?: number;
    audioBytes?: number;
    audioDurationMs?: number;
    audioDuration?: {
        totalMs?: number;
        hours?: number;
        minutes?: number;
        seconds?: number;
        formatted?: string;
    };
}

export interface CollectorSettings {
    hasApiKey: boolean;
    configured: boolean;
    connected: boolean;
    collectorURL?: string;
    serverName?: string;
    serverUrl?: string;
    defaultCollectorURL: string;
}

@Injectable()
export class TranscriptReviewService {
    constructor(
        private http: HttpClient,
        private adminService: RdioScannerAdminService,
    ) {}

    private baseUrl(): string {
        return `${window.location.origin}/api/admin/transcript-review`;
    }

    hasAdminToken(): boolean {
        return !!this.adminService.getToken();
    }

    readUserPin(): string | undefined {
        try {
            const pin = window?.localStorage?.getItem(RdioScannerService.LOCAL_STORAGE_KEY_PIN);
            return pin ? window.atob(pin) : undefined;
        } catch {
            return undefined;
        }
    }

    async getCollectorSettings(): Promise<CollectorSettings> {
        return firstValueFrom(
            this.http.get<CollectorSettings>(`${this.baseUrl()}/collector`, {
                headers: this.adminService.getAuthHeaders(),
            }),
        );
    }

    async saveCollectorSettings(collectorAPIKey: string): Promise<void> {
        await firstValueFrom(
            this.http.put(`${this.baseUrl()}/collector`, { collectorAPIKey }, {
                headers: this.adminService.getAuthHeaders(),
            }),
        );
    }

    async requestCollectorKey(): Promise<{ message: string; serverName?: string; serverUrl?: string }> {
        // Same-origin via TLR server — avoids CORS and service-worker issues with cross-origin fetch.
        return firstValueFrom(
            this.http.post<{ message: string; serverName?: string; serverUrl?: string }>(
                `${this.baseUrl()}/collector/request-key`,
                {},
                { headers: this.adminService.getAuthHeaders() },
            ),
        );
    }

    async clearCollectorKey(): Promise<void> {
        await firstValueFrom(
            this.http.delete(`${this.baseUrl()}/collector`, {
                headers: this.adminService.getAuthHeaders(),
            }),
        );
    }

    async getCollectorStats(): Promise<CollectorContributionStats> {
        return firstValueFrom(
            this.http.get<CollectorContributionStats>(`${this.baseUrl()}/collector/stats`, {
                headers: this.adminService.getAuthHeaders(),
            }),
        );
    }

    /** Admin queue first; falls back to GET /api/transcripts when admin queue is empty. */
    async list(offset = 0, limit = 50): Promise<TranscriptReviewListResponse> {
        const hasPin = !!this.readUserPin();
        let collectorConfigured = false;
        let adminError = '';
        let adminItems: TranscriptReviewItem[] = [];

        if (!this.hasAdminToken()) {
            adminError = 'No admin session token — log out and log in again.';
        } else {
            try {
                const params = new URLSearchParams({ offset: String(offset), limit: String(limit) });
                const adminRes = await firstValueFrom(
                    this.http.get<TranscriptReviewListResponse>(`${this.baseUrl()}?${params}`, {
                        headers: this.adminService.getAuthHeaders(),
                    }),
                );
                collectorConfigured = !!adminRes?.collectorConfigured;
                adminItems = adminRes?.items || [];
                if (adminItems.length) {
                    return { ...adminRes, items: adminItems, source: 'admin', hasPin };
                }
            } catch (e: unknown) {
                adminError = this.httpErrorMessage(e);
            }
        }

        let alertsError = '';
        let alertsItems: TranscriptReviewItem[] = [];
        if (!hasPin) {
            alertsError = 'No TLR PIN saved — open the main scanner app and sign in first.';
        } else {
            try {
                alertsItems = await this.listFromAlertsApi(offset, limit);
            } catch (e: unknown) {
                alertsError = this.httpErrorMessage(e);
            }
        }

        return {
            items: alertsItems,
            offset,
            limit,
            collectorConfigured,
            source: alertsItems.length ? 'alerts' : 'none',
            adminError,
            alertsError,
            hasPin,
        };
    }

    private async listFromAlertsApi(offset: number, limit: number): Promise<TranscriptReviewItem[]> {
        const pin = this.readUserPin();
        if (!pin) {
            return [];
        }
        const params = new URLSearchParams({
            limit: String(limit),
            offset: String(offset),
            pin,
        });
        const url = `${window.location.origin}/api/transcripts?${params}`;
        const rows = await firstValueFrom(
            this.http.get<any[]>(url, {
                headers: new HttpHeaders({ Authorization: `Bearer ${pin}` }),
            }),
        );
        return (rows || []).map((t) => this.mapAlertsRow(t));
    }

    private mapAlertsRow(t: any): TranscriptReviewItem {
        const tg = t.talkgroupLabel || t.talkgroupName || '';
        return {
            callId: t.callId,
            timestamp: t.timestamp,
            transcript: t.transcript || '',
            reviewedTranscript: t.reviewedTranscript || t.transcript || '',
            trainingReviewStatus: t.trainingReviewStatus || '',
            transcriptionStatus: t.transcriptionStatus,
            systemLabel: t.systemLabel || '',
            talkgroupLabel: tg,
            talkgroupName: t.talkgroupName,
        };
    }

    private httpErrorMessage(e: unknown): string {
        if (e instanceof HttpErrorResponse) {
            const body = e.error;
            if (body && typeof body === 'object' && body.error) {
                return String(body.error);
            }
            if (typeof body === 'string' && body.trim()) {
                return body.trim();
            }
            return e.message || `HTTP ${e.status}`;
        }
        if (e instanceof Error) {
            return e.message;
        }
        return 'Request failed';
    }

    async save(callId: number, reviewedTranscript: string): Promise<void> {
        await firstValueFrom(
            this.http.put(`${this.baseUrl()}/${callId}`, { reviewedTranscript }, {
                headers: this.adminService.getAuthHeaders(),
            }),
        );
    }

    async approve(callId: number, reviewedTranscript: string): Promise<{ message: string }> {
        return firstValueFrom(
            this.http.post<{ message: string }>(`${this.baseUrl()}/${callId}/approve`, { reviewedTranscript }, {
                headers: this.adminService.getAuthHeaders(),
            }),
        );
    }

    audioUrl(callId: number): string {
        return `${this.baseUrl()}/${callId}/audio`;
    }

    getAudioFetchHeaders(): Record<string, string> {
        return this.adminService.getFetchHeaders();
    }
}
