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

import { CdkDragDrop, moveItemInArray } from '@angular/cdk/drag-drop';
import { Component, Input, QueryList, ViewChildren } from '@angular/core';
import { FormArray, FormGroup } from '@angular/forms';
import { MatExpansionPanel } from '@angular/material/expansion';
import { RdioScannerAdminService, Group, Tag } from '../../admin.service';

@Component({
    selector: 'rdio-scanner-admin-systems',
    templateUrl: './systems.component.html',
})
export class RdioScannerAdminSystemsComponent {
    @Input() form: FormArray | undefined;

    // Pagination and search
    systemsPage: number = 0;
    systemsPageSize: number = 50;
    systemsSearchTerm: string = '';

    // Cached sorted array
    private _cachedSystems: FormGroup[] = [];
    private _lastSystemsVersion: number = 0;

    get systems(): FormGroup[] {
        if (!this.form) return [];
        
        const currentVersion = this.form.length;
        if (this._lastSystemsVersion !== currentVersion || this._cachedSystems.length !== this.form.length) {
            this._cachedSystems = (this.form.controls as FormGroup[])
                .slice()
                .sort((a, b) => (a.value.order || 0) - (b.value.order || 0));
            this._lastSystemsVersion = currentVersion;
        }
        return this._cachedSystems;
    }

    // Filtered and paginated systems
    get filteredSystems(): FormGroup[] {
        let filtered = this.systems;
        if (this.systemsSearchTerm.trim()) {
            const search = this.systemsSearchTerm.toLowerCase();
            filtered = filtered.filter(sys => {
                const label = (sys.value.label || '').toLowerCase();
                const id = (sys.value.systemRef || '').toString();
                return label.includes(search) || id.includes(search);
            });
        }
        return filtered;
    }

    get paginatedSystems(): FormGroup[] {
        const start = this.systemsPage * this.systemsPageSize;
        const end = start + this.systemsPageSize;
        return this.filteredSystems.slice(start, end);
    }

    get groups(): Group[] {
        if (!this.form) return [];
        const groupsArray = this.form.root.get('groups') as FormArray;
        return groupsArray ? groupsArray.value : [];
    }

    get tags(): Tag[] {
        if (!this.form) return [];
        const tagsArray = this.form.root.get('tags') as FormArray;
        return tagsArray ? tagsArray.value : [];
    }

    get apikeys(): any[] {
        if (!this.form) return [];
        const apikeysArray = this.form.root.get('apikeys') as FormArray;
        return apikeysArray ? apikeysArray.value : [];
    }

    @ViewChildren(MatExpansionPanel) private panels: QueryList<MatExpansionPanel> | undefined;

    constructor(private adminService: RdioScannerAdminService) { }

    add(): void {
        const system = this.adminService.newSystemForm();

        system.markAllAsTouched();

        this.form?.insert(0, system);

        this.form?.markAsDirty();
        this._lastSystemsVersion++;
        this.systemsPage = 0; // Reset to first page
    }

    closeAll(): void {
        this.panels?.forEach((panel) => panel.close());
    }

    drop(event: CdkDragDrop<FormGroup[]>): void {
        if (event.previousIndex !== event.currentIndex) {
            moveItemInArray(event.container.data, event.previousIndex, event.currentIndex);

            event.container.data.forEach((dat, idx) => dat.get('order')?.setValue(idx + 1, { emitEvent: false }));

            this.form?.markAsDirty();
            this._lastSystemsVersion++;
        }
    }

    remove(index: number): void {
        this.form?.removeAt(index);

        this.form?.markAsDirty();
        this._lastSystemsVersion++;
    }

    removeAll(): void {
        if (!this.form || this.form.length === 0) {
            return;
        }

        const count = this.form.length;
        if (!confirm(`Are you sure you want to delete all ${count} system${count > 1 ? 's' : ''}? This action cannot be undone.`)) {
            return;
        }

        while (this.form.length > 0) {
            this.form.removeAt(0);
        }

        this.form.markAsDirty();
        this._lastSystemsVersion++;
        this.systemsPage = 0;
    }

    getFormErrors(formGroup: FormGroup): string {
        const errors: string[] = [];
        
        // Check each control in the form group
        Object.keys(formGroup.controls).forEach(key => {
            const control = formGroup.get(key);
            if (control && control.invalid) {
                if (key === 'label' && control.hasError('required')) {
                    errors.push('Label required');
                } else if (key === 'systemRef' && control.hasError('required')) {
                    errors.push('System ID required');
                } else if (key === 'systemRef' && control.hasError('duplicate')) {
                    errors.push('Duplicate system ID');
                } else if (key === 'systemRef' && control.hasError('min')) {
                    errors.push('Invalid system ID');
                } else if (key === 'talkgroups' && control.invalid) {
                    // Count invalid talkgroups
                    const talkgroupsArray = control as FormArray;
                    const invalidCount = talkgroupsArray.controls.filter(c => c.invalid).length;
                    if (invalidCount > 0) {
                        errors.push(`${invalidCount} invalid talkgroup${invalidCount > 1 ? 's' : ''}`);
                    }
                } else if (key === 'sites' && control.invalid) {
                    const sitesArray = control as FormArray;
                    const invalidCount = sitesArray.controls.filter(c => c.invalid).length;
                    if (invalidCount > 0) {
                        errors.push(`${invalidCount} invalid site${invalidCount > 1 ? 's' : ''}`);
                    }
                } else if (key === 'units' && control.invalid) {
                    const unitsArray = control as FormArray;
                    const invalidCount = unitsArray.controls.filter(c => c.invalid).length;
                    if (invalidCount > 0) {
                        errors.push(`${invalidCount} invalid unit${invalidCount > 1 ? 's' : ''}`);
                    }
                }
            }
        });

        return errors.join(', ');
    }

    // Pagination methods
    onSystemsSearchChange(searchTerm: string): void {
        this.systemsSearchTerm = searchTerm;
        this.systemsPage = 0; // Reset to first page on search
    }

    onSystemsPageChange(page: number): void {
        this.systemsPage = page;
    }

    get systemsTotalPages(): number {
        return Math.ceil(this.filteredSystems.length / this.systemsPageSize);
    }
}
