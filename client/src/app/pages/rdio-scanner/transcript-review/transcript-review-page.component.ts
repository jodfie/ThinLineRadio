import { Component, OnDestroy, OnInit } from '@angular/core';
import { FormBuilder, FormGroup, Validators } from '@angular/forms';
import { Title } from '@angular/platform-browser';
import { MatSnackBar } from '@angular/material/snack-bar';
import { Subscription } from 'rxjs';
import { AdminEvent, RdioScannerAdminService } from '../../../components/rdio-scanner/admin/admin.service';
import { TranscriptReviewService } from '../../../components/rdio-scanner/transcript-review/transcript-review.service';

@Component({
    selector: 'rdio-scanner-transcript-review-page',
    templateUrl: './transcript-review-page.component.html',
    styleUrls: ['./transcript-review-page.component.scss'],
})
export class RdioScannerTranscriptReviewPageComponent implements OnInit, OnDestroy {
    authenticated = false;
    version = '';
    loginForm: FormGroup;
    loginMessage = '';
    loginLoading = false;

    collectorConfigured = false;
    collectorServerName = '';
    requestingKey = false;
    collectorError = '';

    private eventSubscription: Subscription | undefined;

    constructor(
        private adminService: RdioScannerAdminService,
        private reviewService: TranscriptReviewService,
        private titleService: Title,
        private ngFormBuilder: FormBuilder,
        private snackBar: MatSnackBar,
    ) {
        const params = new URLSearchParams(window.location.search);
        const cmToken = params.get('cm_token');
        const ssoToken = params.get('sso_token');
        if (cmToken) {
            this.adminService.setTokenFromExternal(cmToken);
            window.history.replaceState({}, '', window.location.pathname);
        } else if (ssoToken) {
            this.adminService.setTokenFromExternal(ssoToken);
            window.history.replaceState({}, '', window.location.pathname);
        }

        this.authenticated = this.adminService.authenticated;
        this.loginForm = this.ngFormBuilder.group({
            password: this.ngFormBuilder.control(null, Validators.required),
        });
    }

    ngOnInit(): void {
        document.body.classList.add('tlr-transcripts-page');
        this.titleService.setTitle('Transcript Review - TLR');
        this.adminService.getLoginConfig().then(cfg => {
            if (cfg.version) {
                this.version = cfg.version;
            }
        });

        this.eventSubscription = this.adminService.event.subscribe((ev: AdminEvent) => {
            if (ev.authenticated !== undefined) {
                this.authenticated = ev.authenticated;
                if (ev.authenticated) {
                    void this.ensureCollectorConnected();
                }
            }
        });

        if (this.authenticated) {
            void this.ensureCollectorConnected();
        }
    }

    ngOnDestroy(): void {
        document.body.classList.remove('tlr-transcripts-page');
        this.eventSubscription?.unsubscribe();
    }

    async login(): Promise<void> {
        const password = this.loginForm.get('password')?.value;
        if (!password) {
            return;
        }
        this.loginLoading = true;
        this.loginMessage = '';
        const ok = await this.adminService.login(password);
        this.loginLoading = false;
        if (ok) {
            this.authenticated = true;
            this.loginForm.reset();
            await this.ensureCollectorConnected();
        } else {
            this.loginMessage = 'Invalid password';
        }
    }

    async ssoLogin(): Promise<void> {
        const pin = window.localStorage.getItem('rdio-scanner-pin');
        const decodedPin = pin ? window.atob(pin) : null;
        if (!decodedPin) {
            this.loginMessage = 'No active TLR session. Sign in to TLR first, then return here.';
            return;
        }
        this.loginLoading = true;
        this.loginMessage = '';
        const ok = await this.adminService.ssoLogin(decodedPin);
        this.loginLoading = false;
        if (ok) {
            this.authenticated = true;
            await this.ensureCollectorConnected();
        } else {
            this.loginMessage = 'SSO login failed. Your account needs System Admin privileges.';
        }
    }

    logout(): void {
        this.adminService.logout();
        this.authenticated = false;
    }

    async ensureCollectorConnected(): Promise<void> {
        this.collectorError = '';
        try {
            const collector = await this.reviewService.getCollectorSettings();
            this.collectorConfigured = !!collector.configured;
            if (this.collectorConfigured) {
                return;
            }
        } catch {
            // fall through to one-click register
        }
        await this.requestCollectorKey();
    }

    async requestCollectorKey(): Promise<void> {
        if (this.requestingKey) {
            return;
        }
        this.requestingKey = true;
        this.collectorError = '';
        try {
            const res = await this.reviewService.requestCollectorKey();
            this.collectorConfigured = true;
            this.collectorServerName = res.serverName || '';
            this.snackBar.open(res.message || 'Connected to transcript collector', '', { duration: 4000 });
        } catch (e: any) {
            this.collectorConfigured = false;
            this.collectorError = e?.error?.error || e?.message || 'Could not connect to transcript collector';
        } finally {
            this.requestingKey = false;
        }
    }
}
