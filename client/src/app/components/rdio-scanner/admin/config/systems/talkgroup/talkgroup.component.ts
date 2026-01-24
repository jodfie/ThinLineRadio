/*
 * *****************************************************************************
 * Copyright (C) 2019-2024 Chrystian Huot <chrystian@huot.qc.ca>
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

import { Component, ElementRef, EventEmitter, Input, Output, ViewChild } from '@angular/core';
import { FormArray, FormBuilder, FormGroup, Validators } from '@angular/forms';
import { MatSelectChange } from '@angular/material/select';
import { MatSnackBar } from '@angular/material/snack-bar';
import { finalize } from 'rxjs/operators';
import { RdioScannerAdminService, Group, Tag } from '../../../admin.service';
import { RdioScannerToneSet } from '../../../../rdio-scanner';

@Component({
    selector: 'rdio-scanner-admin-talkgroup',
    templateUrl: './talkgroup.component.html',
})
export class RdioScannerAdminTalkgroupComponent {
    @Input() form: FormGroup | undefined;

    @Output() blacklist = new EventEmitter<void>();

    @Output() remove = new EventEmitter<void>();

    @ViewChild('twoToneFileInput') twoToneFileInput?: ElementRef<HTMLInputElement>;
    @ViewChild('csvFileInput') csvFileInput?: ElementRef<HTMLInputElement>;

    importingToneSets = false;

    get groups(): Group[] {
        return this.form?.root.get('groups')?.value as Group[];
    }

    get tags(): Tag[] {
        return this.form?.root.get('tags')?.value as Tag[];
    }

    get apikeys(): any[] {
        return this.form?.root.get('apikeys')?.value as any[] || [];
    }

    constructor(
        private adminService: RdioScannerAdminService,
        private formBuilder: FormBuilder,
        private snackBar: MatSnackBar,
    ) {
    }

    getToneSets(): FormArray {
        if (!this.form) {
            return this.formBuilder.array([]) as FormArray;
        }
        let toneSetsArray = this.form.get('toneSets') as FormArray;
        if (!toneSetsArray) {
            toneSetsArray = this.formBuilder.array([]);
            this.form.addControl('toneSets', toneSetsArray);
        }
        return toneSetsArray;
    }

    addToneSet(toneSet?: Partial<RdioScannerToneSet>): void {
        const toneSetForm = this.formBuilder.group({
            id: [toneSet?.id || this.generateToneSetId()],
            label: [toneSet?.label || '', Validators.required],
            aToneFrequency: [toneSet?.aTone?.frequency ?? null],
            aToneMinDuration: [toneSet?.aTone?.minDuration ?? null],
            aToneMaxDuration: [toneSet?.aTone?.maxDuration ?? null],
            bToneFrequency: [toneSet?.bTone?.frequency ?? null],
            bToneMinDuration: [toneSet?.bTone?.minDuration ?? null],
            bToneMaxDuration: [toneSet?.bTone?.maxDuration ?? null],
            longToneFrequency: [toneSet?.longTone?.frequency ?? null],
            longToneMinDuration: [toneSet?.longTone?.minDuration ?? null],
            longToneMaxDuration: [toneSet?.longTone?.maxDuration ?? null],
            tolerance: [toneSet?.tolerance ?? 10],
            minDuration: [toneSet?.minDuration ?? null],
        });
        this.getToneSets().push(toneSetForm);
    }

    removeToneSet(index: number): void {
        this.getToneSets().removeAt(index);
    }

    triggerToneImport(format: ToneImportFormat): void {
        if (format === 'twotone') {
            this.twoToneFileInput?.nativeElement.click();
        } else {
            this.csvFileInput?.nativeElement.click();
        }
    }

    async handleToneImport(event: Event, format: ToneImportFormat): Promise<void> {
        const input = event.target as HTMLInputElement;
        const file = input?.files?.[0];
        if (!file || !this.form) {
            return;
        }

        let content = '';
        try {
            content = await file.text();
        } catch {
            this.snackBar.open('Unable to read the selected file', '', { duration: 4000 });
            input.value = '';
            return;
        }

        this.importingToneSets = true;
        this.adminService.importToneSets(format, content)
            .pipe(finalize(() => {
                this.importingToneSets = false;
                if (input) {
                    input.value = '';
                }
            }))
            .subscribe({
                next: (response) => {
                    const imported = response?.toneSets || [];
                    if (imported.length > 0) {
                        this.appendImportedToneSets(imported);
                        const label = format === 'twotone' ? 'TwoToneDetect' : 'CSV';
                        this.snackBar.open(`Imported ${imported.length} tone set${imported.length === 1 ? '' : 's'} from ${label}`, '', { duration: 4000 });
                    } else {
                        this.snackBar.open('No tone sets were found in the selected file', '', { duration: 5000 });
                    }

                    if (response?.warnings?.length) {
                        this.snackBar.open(response.warnings.join(' '), 'Dismiss', { duration: 6000 });
                    }
                },
                error: (error) => {
                    const message = error?.error?.error || 'Failed to import tone sets';
                    this.snackBar.open(message, '', { duration: 6000 });
                },
            });
    }

    private appendImportedToneSets(toneSets: RdioScannerToneSet[]): void {
        if (!this.form) {
            return;
        }

        if (!this.form.get('toneDetectionEnabled')?.value) {
            this.form.get('toneDetectionEnabled')?.setValue(true);
        }

        toneSets.forEach((toneSet) => this.addToneSet(toneSet));
    }

    private generateToneSetId(): string {
        return `tone-set-${Date.now()}-${Math.random().toString(36).substr(2, 9)}`;
    }
}

type ToneImportFormat = 'twotone' | 'csv';
