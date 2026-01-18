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

import { Component, OnDestroy, OnInit } from '@angular/core';
import { RdioScannerAlert, RdioScannerCall, RdioScannerService, RdioScannerTranscript } from '../rdio-scanner';
import { AlertsService } from './alerts.service';
import { AlertSoundService } from '../alert-sound.service';
import { SettingsService } from '../settings/settings.service';

@Component({
    selector: 'rdio-scanner-alerts',
    styleUrls: ['./alerts.component.scss'],
    templateUrl: './alerts.component.html',
})
export class RdioScannerAlertsComponent implements OnDestroy, OnInit {
    alerts: RdioScannerAlert[] = [];
    transcripts: RdioScannerTranscript[] = [];
    loading = false;
    loadingTranscripts = false;
    limit = 50;
    transcriptOffset = 0;
    activeTab: 'alerts' | 'preferences' | 'transcripts' = 'alerts';
    private pin?: string;
    
    // Filter properties
    filterSystemId?: number;
    filterTalkgroupId?: number;
    filterDateFrom?: string; // YYYY-MM-DD format for date input
    filterDateTo?: string; // YYYY-MM-DD format for date input
    filterSearch: string = '';
    availableSystems: Array<{id: number, label: string}> = [];
    availableTalkgroups: Array<{id: number, label: string, systemId: number}> = [];
    
    // Cached grouped alerts to avoid recalculation on every change detection
    allAlertGroups: Array<{key: string, alerts: RdioScannerAlert[], latestTimestamp: number, groupType: 'tone' | 'channel'}> = [];

    constructor(
        private rdioScannerService: RdioScannerService,
        private alertsService: AlertsService,
        private alertSoundService: AlertSoundService,
        private settingsService: SettingsService,
    ) {
        // Get PIN from localStorage using the service method
        this.pin = this.rdioScannerService.readPin();
    }

    ngOnInit(): void {
        // Refresh PIN from localStorage
        this.pin = this.rdioScannerService.readPin();
        
        // Load systems and talkgroups from config
        this.loadSystemsAndTalkgroups();
        
        // Initial full load
        this.loadAlerts(true);
        this.loadTranscripts();
        this.requestNotificationPermission();
        
        // Subscribe to shared alerts service for updates
        this.alertsService.alerts$.subscribe(alerts => {
            this.alerts = alerts;
            this.updateGroupedAlerts();
        });
        
        // Listen for real-time alerts via WebSocket
        this.rdioScannerService.event.subscribe((event: any) => {
            if (event.alert) {
                // Fetch only new alerts (incremental)
                this.loadAlerts(false);
                this.loadTranscripts();
                this.showNotification(event.alert);
                this.playAlertSound();
            }
            if (event.config) {
                // Config updated, reload systems/talkgroups
                this.loadSystemsAndTalkgroups();
            }
        });
    }
    
    loadSystemsAndTalkgroups(): void {
        const config = this.rdioScannerService.getConfig();
        if (config && config.systems) {
            this.availableSystems = config.systems.map(s => ({
                id: s.id,
                label: s.label || `System ${s.id}`
            }));
            
            // Flatten talkgroups from all systems
            this.availableTalkgroups = [];
            config.systems.forEach(system => {
                if (system.talkgroups) {
                    system.talkgroups.forEach(tg => {
                        this.availableTalkgroups.push({
                            id: tg.id,
                            label: tg.label || tg.name || `Talkgroup ${tg.id}`,
                            systemId: system.id
                        });
                    });
                }
            });
        }
    }
    
    getFilteredTalkgroups(): Array<{id: number, label: string, systemId: number}> {
        if (!this.filterSystemId) {
            return this.availableTalkgroups;
        }
        return this.availableTalkgroups.filter(tg => tg.systemId === this.filterSystemId);
    }
    
    onSystemFilterChange(value: any): void {
        // Convert to number if it's a string, or set to undefined if empty/null
        if (value === '' || value === null || value === undefined || value === 'undefined') {
            this.filterSystemId = undefined;
        } else {
            const numValue = typeof value === 'string' ? parseInt(value, 10) : Number(value);
            this.filterSystemId = isNaN(numValue) ? undefined : numValue;
        }
        // Reset talkgroup filter when system changes
        this.filterTalkgroupId = undefined;
        this.applyFilters();
    }
    
    onTalkgroupFilterChange(value: any): void {
        // Convert to number if it's a string, or set to undefined if empty/null
        if (value === '' || value === null || value === undefined || value === 'undefined') {
            this.filterTalkgroupId = undefined;
        } else {
            const numValue = typeof value === 'string' ? parseInt(value, 10) : Number(value);
            this.filterTalkgroupId = isNaN(numValue) ? undefined : numValue;
        }
        this.applyFilters();
    }
    
    applyFilters(): void {
        // Reset to first page when filters change
        this.transcriptOffset = 0;
        this.loadTranscripts();
    }
    
    clearFilters(): void {
        this.filterSystemId = undefined;
        this.filterTalkgroupId = undefined;
        this.filterDateFrom = undefined;
        this.filterDateTo = undefined;
        this.filterSearch = '';
        this.applyFilters();
    }
    
    highlightSearchText(text: string, search: string): string {
        if (!search || !text) {
            return text;
        }
        const regex = new RegExp(`(${search.replace(/[.*+?^${}()|[\]\\]/g, '\\$&')})`, 'gi');
        return text.replace(regex, '<mark>$1</mark>');
    }

    ngOnDestroy(): void {
    }
    setTab(tab: 'alerts' | 'preferences' | 'transcripts'): void {
        this.activeTab = tab;
        if (tab === 'alerts') {
            // Only fetch new alerts when switching to alerts tab (incremental)
            this.loadAlerts(false);
        } else if (tab === 'transcripts') {
            this.loadTranscripts();
        }
    }


    nextTranscriptsPage(): void {
        this.transcriptOffset += this.limit;
        this.loadTranscripts();
    }

    prevTranscriptsPage(): void {
        this.transcriptOffset = Math.max(0, this.transcriptOffset - this.limit);
        this.loadTranscripts();
    }


    loadAlerts(forceFullRefresh: boolean = false): void {
        // Refresh PIN before each request
        this.pin = this.rdioScannerService.readPin();
        
        if (!this.pin) {
            console.warn('No PIN available for loading alerts');
            this.loading = false;
            this.alerts = [];
            this.updateGroupedAlerts();
            return;
        }
        
        this.loading = true;
        
        // Use shared service to fetch new alerts incrementally
        this.alertsService.fetchNewAlerts(this.pin, forceFullRefresh).subscribe({
            next: (newAlerts) => {
                // Get all alerts from cache (includes new ones)
                this.alerts = this.alertsService.getCachedAlerts();
                
                this.updateGroupedAlerts();
                
                this.loading = false;
            },
            error: (error) => {
                console.error('Error loading alerts:', error);
                // On error, still try to use cached alerts
                this.alerts = this.alertsService.getCachedAlerts();
                this.updateGroupedAlerts();
                this.loading = false;
            },
        });
    }

    loadTranscripts(): void {
        this.pin = this.rdioScannerService.readPin();
        if (!this.pin) {
            this.transcripts = [];
            return;
        }
        this.loadingTranscripts = true;
        
        // Convert date strings (YYYY-MM-DD) to timestamps (start of day for from, end of day for to)
        let dateFrom: number | undefined;
        let dateTo: number | undefined;
        if (this.filterDateFrom) {
            const date = new Date(this.filterDateFrom + 'T00:00:00');
            dateFrom = Math.floor(date.getTime() / 1000) * 1000;
        }
        if (this.filterDateTo) {
            const date = new Date(this.filterDateTo + 'T23:59:59');
            dateTo = Math.floor(date.getTime() / 1000) * 1000;
        }
        
        this.alertsService.getTranscripts(
            this.limit, 
            this.transcriptOffset, 
            this.pin, 
            this.filterSystemId, 
            this.filterTalkgroupId,
            dateFrom,
            dateTo,
            this.filterSearch
        ).subscribe({
            next: (transcripts) => {
                console.log('Received transcripts:', transcripts?.length, 'transcripts');
                this.transcripts = (transcripts || []).map((t: any) => {
                    return {
                        ...t,
                        transcript: t.transcript || '',
                    } as RdioScannerTranscript;
                });
                this.loadingTranscripts = false;
            },
            error: (error) => {
                console.error('Error loading transcripts:', error);
                this.transcripts = [];
                this.loadingTranscripts = false;
            },
        });
    }

    getAlertTypeLabel(alert: RdioScannerAlert): string {
        switch (alert.alertType) {
            case 'tone':
                return 'Tone Detected';
            case 'keyword':
                return 'Keyword Match';
            case 'tone+keyword':
                return 'Tone & Keyword';
            default:
                return 'Alert';
        }
    }

    getKeywordsMatched(alert: RdioScannerAlert): string[] {
        if (!alert.keywordsMatched) {
            return [];
        }
        try {
            return JSON.parse(alert.keywordsMatched);
        } catch {
            return [];
        }
    }

    formatTimestamp(timestamp: number): string {
        const date = new Date(timestamp);
        const datePart = date.toLocaleDateString();
        const timePart = date.toLocaleTimeString();
        const spacer = '\u00A0\u00A0\u00A0'; // three non-breaking spaces
        return `${datePart}${spacer}${timePart}`;
    }

    // Update cached grouped alerts (called when alerts change to avoid recalculation on every change detection)
    private updateGroupedAlerts(): void {
        // Group tone alerts by tone set name
        const toneGrouped = new Map<string, RdioScannerAlert[]>();
        
        this.alerts.filter(alert => 
            alert.alertType === 'tone' || alert.alertType === 'tone+keyword'
        ).forEach(alert => {
            // Get tone set name from alert - prefer matchedToneSetName (specific tone set for this alert)
            // then fall back to first tone set from matchedToneSetNames
            let toneSetKey = 'Unknown Tone Set';
            if (alert.matchedToneSetName) {
                toneSetKey = alert.matchedToneSetName;
            } else if (alert.matchedToneSetNames && alert.matchedToneSetNames.length > 0) {
                toneSetKey = alert.matchedToneSetNames[0];
            }
            
            if (!toneGrouped.has(toneSetKey)) {
                toneGrouped.set(toneSetKey, []);
            }
            toneGrouped.get(toneSetKey)!.push(alert);
        });
        
        // Convert to array and find latest timestamp for each group
        const toneGroups = Array.from(toneGrouped.entries()).map(([key, alerts]) => {
            // Find the most recent alert timestamp in this group
            const latestTimestamp = Math.max(...alerts.map(a => a.createdAt || 0));
            return {
                key,
                alerts,
                latestTimestamp,
                groupType: 'tone' as const
            };
        });

        // Group channel alerts by channel (system + talkgroup)
        const channelGrouped = new Map<string, RdioScannerAlert[]>();
        
        this.alerts.filter(alert => alert.alertType === 'keyword').forEach(alert => {
            // Create channel key from system + talkgroup
            const channelKey = `${alert.systemLabel || `System ${alert.systemId}`} / ${alert.talkgroupLabel || alert.talkgroupName || `Talkgroup ${alert.talkgroupId}`}`;
            
            if (!channelGrouped.has(channelKey)) {
                channelGrouped.set(channelKey, []);
            }
            channelGrouped.get(channelKey)!.push(alert);
        });
        
        // Convert to array and find latest timestamp for each group
        const channelGroups = Array.from(channelGrouped.entries()).map(([key, alerts]) => {
            // Find the most recent alert timestamp in this group
            const latestTimestamp = Math.max(...alerts.map(a => a.createdAt || 0));
            return {
                key,
                alerts,
                latestTimestamp,
                groupType: 'channel' as const
            };
        });
        
        // Combine all groups and sort by most recent alert timestamp
        this.allAlertGroups = [...toneGroups, ...channelGroups].sort((a, b) => b.latestTimestamp - a.latestTimestamp);
    }

    // TrackBy functions for efficient change detection
    trackByGroupKey(index: number, group: {key: string, alerts: RdioScannerAlert[], latestTimestamp: number}): string {
        return group.key;
    }

    trackByAlertId(index: number, alert: RdioScannerAlert): number {
        return alert.alertId;
    }

    playCall(callId: number): void {
        // Trigger call playback
        this.rdioScannerService.loadAndPlay(callId);
    }

    requestNotificationPermission(): void {
        if ('Notification' in window && Notification.permission === 'default') {
            Notification.requestPermission();
        }
    }

    showNotification(alert: RdioScannerAlert): void {
        if ('Notification' in window && Notification.permission === 'granted') {
            const keywords = this.getKeywordsMatched(alert);
            const keywordText = keywords.length > 0 ? `Keywords: ${keywords.join(', ')}` : '';
            const notification = new Notification(
                this.getAlertTypeLabel(alert),
                {
                    body: alert.transcriptSnippet || keywordText || 'Alert detected',
                    icon: '/assets/icons/icon.png',
                    tag: `alert-${alert.alertId}`,
                }
            );
            notification.onclick = () => {
                this.playCall(alert.callId);
                window.focus();
            };
        }
    }

    private playAlertSound(): void {
        // Get the alert sound setting and play it
        this.settingsService.getSettings().subscribe({
            next: (settings) => {
                const alertSound = settings?.alertSound || 'alert';
                this.alertSoundService.playSound(alertSound);
            },
            error: (error) => {
                console.error('Failed to get alert sound setting:', error);
                // Play default alert sound on error
                this.alertSoundService.playSound('alert');
            }
        });
    }
}

