import { NgModule } from '@angular/core';
import { FormsModule, ReactiveFormsModule } from '@angular/forms';
import { MatButtonModule } from '@angular/material/button';
import { MatFormFieldModule } from '@angular/material/form-field';
import { MatIconModule } from '@angular/material/icon';
import { MatInputModule } from '@angular/material/input';
import { MatProgressSpinnerModule } from '@angular/material/progress-spinner';
import { MatSnackBarModule } from '@angular/material/snack-bar';
import { RdioScannerAdminService } from '../../../components/rdio-scanner/admin/admin.service';
import { TranscriptReviewService } from '../../../components/rdio-scanner/transcript-review/transcript-review.service';
import { RdioScannerTranscriptReviewModule } from '../../../components/rdio-scanner/transcript-review/transcript-review.module';
import { AppSharedModule } from '../../../shared/shared.module';
import { RdioScannerTranscriptReviewPageComponent } from './transcript-review-page.component';
import { routes } from './transcript-review.routes';

@NgModule({
    declarations: [RdioScannerTranscriptReviewPageComponent],
    imports: [
        AppSharedModule.forChild({ routerRoutes: routes }),
        FormsModule,
        ReactiveFormsModule,
        MatButtonModule,
        MatFormFieldModule,
        MatIconModule,
        MatInputModule,
        MatProgressSpinnerModule,
        MatSnackBarModule,
        RdioScannerTranscriptReviewModule,
    ],
    providers: [RdioScannerAdminService, TranscriptReviewService],
})
export class RdioScannerTranscriptReviewPageModule {}
