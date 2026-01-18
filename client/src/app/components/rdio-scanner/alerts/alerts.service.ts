/*
 * *****************************************************************************
 * Copyright (C) 2025 Thinline Dynamic Solutions
 *
 * This program is free software: you can redistribute it and/or modify
 * it under the terms of the GNU General Public License as published by
 * the Free Software Foundation, either version 3 of the License, or
 * (at your option) any later version.
 *
 * This program is distributed in the hope that it will be useful,
 * but WITHOUT ANY WARRANTY; without even the implied warranty of
 * MERCHANTABILITY or FITNESS FOR A PARTICULAR PURPOSE.  See the
 * GNU General Public License for more details.
 *
 * You should have received a copy of the GNU General Public License
 * along with this program.  If not, see <http://www.gnu.org/licenses/>
 * ****************************************************************************
 */

import { HttpClient, HttpHeaders } from '@angular/common/http';
import { Injectable } from '@angular/core';
import { Observable, BehaviorSubject } from 'rxjs';
import { RdioScannerAlert, RdioScannerAlertPreference, RdioScannerKeywordList } from '../rdio-scanner';

@Injectable()
export class AlertsService {
    private readonly apiUrl = '/api/alerts';
    private readonly preferencesUrl = '/api/alerts/preferences';
    private readonly keywordListsUrl = '/api/keyword-lists';
    private readonly transcriptsUrl = '/api/transcripts';

    // Shared alerts cache - single source of truth
    private alertsCache: RdioScannerAlert[] = [];
    private lastFetchTime: number = 0;
    private isFetching: boolean = false;
    private alertsSubject = new BehaviorSubject<RdioScannerAlert[]>([]);
    public alerts$ = this.alertsSubject.asObservable();

    constructor(private http: HttpClient) {
    }

    /**
     * Get all alerts (full fetch - used for initial load)
     */
    getAlerts(limit: number = 50, offset: number = 0, pin?: string): Observable<any[]> {
        let url = `${this.apiUrl}?limit=${limit}&offset=${offset}`;
        if (pin) {
            url += `&pin=${encodeURIComponent(pin)}`;
        }
        const headers = pin ? new HttpHeaders().set('Authorization', `Bearer ${pin}`) : undefined;
        return this.http.get<any[]>(url, { headers });
    }

    /**
     * Fetch new alerts since last fetch time and append to cache
     * Returns the new alerts that were added
     */
    fetchNewAlerts(pin?: string, forceFullRefresh: boolean = false): Observable<RdioScannerAlert[]> {
        // Prevent concurrent fetches - return cached alerts if already fetching
        if (this.isFetching) {
            return new Observable<RdioScannerAlert[]>(observer => {
                // Return empty array if already fetching (will get updates via alerts$ subscription)
                observer.next([]);
                observer.complete();
            });
        }

        this.isFetching = true;

        let url = this.apiUrl;
        const params: string[] = [];

        // If not forcing full refresh and we have a last fetch time, only get new alerts
        if (!forceFullRefresh && this.lastFetchTime > 0) {
            params.push(`since=${this.lastFetchTime}`);
        }

        if (pin) {
            params.push(`pin=${encodeURIComponent(pin)}`);
        }

        if (params.length > 0) {
            url += '?' + params.join('&');
        }

        const headers = pin ? new HttpHeaders().set('Authorization', `Bearer ${pin}`) : undefined;
        
        return new Observable<RdioScannerAlert[]>(observer => {
            this.http.get<any[]>(url, { headers }).subscribe({
                next: (newAlerts) => {
                    try {
                        const alertsToAdd: RdioScannerAlert[] = [];

                        if (forceFullRefresh) {
                            // Replace all alerts
                            this.alertsCache = (newAlerts || []).map((alert: any) => {
                                const typedAlert = alert as RdioScannerAlert;
                                typedAlert.transcriptSnippet = typedAlert.transcriptSnippet || typedAlert.transcript || '';
                                return typedAlert;
                            });
                            
                            // Set lastFetchTime to most recent alert's createdAt, or current time if no alerts
                            if (this.alertsCache.length > 0) {
                                // Alerts are returned in descending order (most recent first)
                                this.lastFetchTime = this.alertsCache[0].createdAt || Date.now();
                            } else {
                                this.lastFetchTime = Date.now();
                            }
                        } else {
                            // Convert and deduplicate
                            const existingIds = new Set(this.alertsCache.map(a => a.alertId));
                            
                            for (const alert of newAlerts || []) {
                                const typedAlert = alert as RdioScannerAlert;
                                typedAlert.transcriptSnippet = typedAlert.transcriptSnippet || typedAlert.transcript || '';
                                
                                // Only add if not already in cache
                                if (!existingIds.has(typedAlert.alertId)) {
                                    alertsToAdd.push(typedAlert);
                                }
                            }

                            // Append new alerts to the beginning (most recent first)
                            this.alertsCache = [...alertsToAdd, ...this.alertsCache];
                            
                            // Update lastFetchTime to most recent alert's createdAt
                            // If we got new alerts, use the first one (most recent), otherwise keep current time
                            if (alertsToAdd.length > 0) {
                                this.lastFetchTime = alertsToAdd[0].createdAt || this.lastFetchTime;
                            } else if (this.alertsCache.length > 0) {
                                // No new alerts, but we have cached ones - use the most recent cached alert
                                this.lastFetchTime = this.alertsCache[0].createdAt || this.lastFetchTime;
                            } else {
                                // No alerts at all - use current time
                                this.lastFetchTime = Date.now();
                            }
                        }

                        // Emit updated alerts
                        this.alertsSubject.next([...this.alertsCache]);

                        this.isFetching = false;
                        observer.next(forceFullRefresh ? this.alertsCache : alertsToAdd);
                        observer.complete();
                    } catch (error) {
                        console.error('Error processing alerts:', error);
                        this.isFetching = false;
                        observer.error(error);
                    }
                },
                error: (error) => {
                    console.error('Error fetching new alerts:', error);
                    this.isFetching = false;
                    observer.error(error);
                }
            });
        });
    }

    /**
     * Get current alerts from cache
     */
    getCachedAlerts(): RdioScannerAlert[] {
        return [...this.alertsCache];
    }

    /**
     * Clear cache and reset
     */
    clearCache(): void {
        this.alertsCache = [];
        this.lastFetchTime = 0;
        this.alertsSubject.next([]);
    }

    getTranscripts(limit: number = 50, offset: number = 0, pin?: string, systemId?: number, talkgroupId?: number, dateFrom?: number, dateTo?: number, search?: string): Observable<any[]> {
        let url = `${this.transcriptsUrl}?limit=${limit}&offset=${offset}`;
        if (pin) {
            url += `&pin=${encodeURIComponent(pin)}`;
        }
        if (systemId !== undefined && systemId !== null) {
            url += `&systemId=${systemId}`;
        }
        if (talkgroupId !== undefined && talkgroupId !== null) {
            url += `&talkgroupId=${talkgroupId}`;
        }
        if (dateFrom) {
            url += `&dateFrom=${dateFrom}`;
        }
        if (dateTo) {
            url += `&dateTo=${dateTo}`;
        }
        if (search && search.trim()) {
            url += `&search=${encodeURIComponent(search.trim())}`;
        }
        const headers = pin ? new HttpHeaders().set('Authorization', `Bearer ${pin}`) : undefined;
        return this.http.get<any[]>(url, { headers });
    }

    getPreferences(pin?: string): Observable<RdioScannerAlertPreference[]> {
        let url = this.preferencesUrl;
        if (pin) {
            url += `?pin=${encodeURIComponent(pin)}`;
    }
        const headers = pin ? new HttpHeaders().set('Authorization', `Bearer ${pin}`) : undefined;
        return this.http.get<RdioScannerAlertPreference[]>(url, { headers });
    }

    updatePreferences(preferences: RdioScannerAlertPreference[], pin?: string): Observable<any> {
        const headers = pin ? new HttpHeaders().set('Authorization', `Bearer ${pin}`) : undefined;
        return this.http.put<any>(this.preferencesUrl, preferences, { headers });
    }

    getKeywordLists(pin?: string): Observable<RdioScannerKeywordList[]> {
        let url = this.keywordListsUrl;
        if (pin) {
            url += `?pin=${encodeURIComponent(pin)}`;
        }
        const headers = pin ? new HttpHeaders().set('Authorization', `Bearer ${pin}`) : undefined;
        return this.http.get<RdioScannerKeywordList[]>(url, { headers });
    }

    createKeywordList(list: Partial<RdioScannerKeywordList>, pin?: string): Observable<RdioScannerKeywordList> {
        const headers = pin ? new HttpHeaders().set('Authorization', `Bearer ${pin}`) : undefined;
        return this.http.post<RdioScannerKeywordList>(this.keywordListsUrl, list, { headers });
    }

    updateKeywordList(listId: number, list: Partial<RdioScannerKeywordList>, pin?: string): Observable<any> {
        const headers = pin ? new HttpHeaders().set('Authorization', `Bearer ${pin}`) : undefined;
        return this.http.put<any>(`${this.keywordListsUrl}/${listId}`, list, { headers });
    }

    deleteKeywordList(listId: number, pin?: string): Observable<any> {
        const headers = pin ? new HttpHeaders().set('Authorization', `Bearer ${pin}`) : undefined;
        return this.http.delete<any>(`${this.keywordListsUrl}/${listId}`, { headers });
    }
}

