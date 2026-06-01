import { HttpClientModule } from '@angular/common/http';
import { NgModule } from '@angular/core';
import { FormsModule } from '@angular/forms';
import { MatButtonModule } from '@angular/material/button';
import { MatIconModule } from '@angular/material/icon';
import { MatSnackBarModule } from '@angular/material/snack-bar';
import { AlertsService } from '../alerts/alerts.service';
import { RdioScannerTranscriptReviewComponent } from './transcript-review.component';
import { RdioScannerTranscriptTrainingTipsComponent } from './transcript-training-tips.component';

@NgModule({
    declarations: [RdioScannerTranscriptReviewComponent, RdioScannerTranscriptTrainingTipsComponent],
    exports: [RdioScannerTranscriptReviewComponent, RdioScannerTranscriptTrainingTipsComponent],
    imports: [
        FormsModule,
        HttpClientModule,
        MatButtonModule,
        MatIconModule,
        MatSnackBarModule,
    ],
    providers: [AlertsService],
})
export class RdioScannerTranscriptReviewModule {}
