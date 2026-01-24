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
import { Component, EventEmitter, Input, Output, QueryList, ViewChildren } from '@angular/core';
import { FormArray, FormControl, FormGroup } from '@angular/forms';
import { MatExpansionPanel } from '@angular/material/expansion';
import { MatSelectChange } from '@angular/material/select';
import { RdioScannerAdminService, Group, Tag } from '../../../admin.service';

@Component({
    selector: 'rdio-scanner-admin-system',
    templateUrl: './system.component.html',
})
export class RdioScannerAdminSystemComponent {
    @Input() form = new FormGroup({});

    @Input() groups: Group[] = [];

    @Input() tags: Tag[] = [];

    @Input() apikeys: any[] = [];  // API keys for preferred API key dropdown

    @Output() add = new EventEmitter<void>();

    @Output() remove = new EventEmitter<void>();

    // Bulk selection state
    selectedTalkgroupIndices: Set<number> = new Set();
    bulkAssignGroupId: number | null = null;
    bulkAssignTagId: number | null = null;

    // Pagination and search
    talkgroupsPage: number = 0;
    talkgroupsPageSize: number = 50;
    talkgroupsSearchTerm: string = '';
    
    unitsPage: number = 0;
    unitsPageSize: number = 50;
    unitsSearchTerm: string = '';
    
    sitesPage: number = 0;
    sitesPageSize: number = 50;
    sitesSearchTerm: string = '';

    // Cached sorted arrays
    private _cachedSites: FormGroup[] = [];
    private _cachedTalkgroups: FormGroup[] = [];
    private _cachedUnits: FormGroup[] = [];
    private _lastSitesVersion: number = 0;
    private _lastTalkgroupsVersion: number = 0;
    private _lastUnitsVersion: number = 0;

    get sites(): FormGroup[] {
        const sitesArray = this.form.get('sites') as FormArray | null;
        if (!sitesArray) return [];
        
        // Check if array changed by comparing length and a version number
        const currentVersion = sitesArray.length;
        if (this._lastSitesVersion !== currentVersion || this._cachedSites.length !== sitesArray.length) {
            // Sort with stable sort: primary by order, secondary by siteId
            this._cachedSites = (sitesArray.controls as FormGroup[])
                .slice()
                .sort((a, b) => {
                    const orderA = a.value.order || 0;
                    const orderB = b.value.order || 0;
                    if (orderA !== orderB) {
                        return orderA - orderB;
                    }
                    // Secondary sort by id to ensure stable sort
                    return (a.value.id || 0) - (b.value.id || 0);
                });
            
            // Reorder the actual FormArray to match display order
            sitesArray.clear({ emitEvent: false });
            this._cachedSites.forEach(control => {
                sitesArray.push(control, { emitEvent: false });
            });
            
            this._lastSitesVersion = currentVersion;
        }
        return this._cachedSites;
    }

    get talkgroups(): FormGroup[] {
        const talkgroupsArray = this.form.get('talkgroups') as FormArray | null;
        if (!talkgroupsArray) return [];
        
        const currentVersion = talkgroupsArray.length;
        if (this._lastTalkgroupsVersion !== currentVersion || this._cachedTalkgroups.length !== talkgroupsArray.length) {
            // Sort with stable sort: primary by order, secondary by talkgroupId to ensure consistent ordering
            this._cachedTalkgroups = (talkgroupsArray.controls as FormGroup[])
                .slice()
                .sort((a, b) => {
                    const orderA = a.value.order || 0;
                    const orderB = b.value.order || 0;
                    if (orderA !== orderB) {
                        return orderA - orderB;
                    }
                    // Secondary sort by talkgroupId to ensure stable sort
                    return (a.value.talkgroupId || 0) - (b.value.talkgroupId || 0);
                });
            
            // CRITICAL FIX: Reorder the actual FormArray to match display order
            // This ensures getRawValue() returns talkgroups in the correct order when saving
            talkgroupsArray.clear({ emitEvent: false });
            this._cachedTalkgroups.forEach(control => {
                talkgroupsArray.push(control, { emitEvent: false });
            });
            
            this._lastTalkgroupsVersion = currentVersion;
        }
        return this._cachedTalkgroups;
    }

    get units(): FormGroup[] {
        const unitsArray = this.form.get('units') as FormArray | null;
        if (!unitsArray) return [];
        
        const currentVersion = unitsArray.length;
        if (this._lastUnitsVersion !== currentVersion || this._cachedUnits.length !== unitsArray.length) {
            // Sort with stable sort: primary by order, secondary by id
            this._cachedUnits = (unitsArray.controls as FormGroup[])
                .slice()
                .sort((a, b) => {
                    const orderA = a.value.order || 0;
                    const orderB = b.value.order || 0;
                    if (orderA !== orderB) {
                        return orderA - orderB;
                    }
                    // Secondary sort by id to ensure stable sort
                    return (a.value.id || 0) - (b.value.id || 0);
                });
            
            // Reorder the actual FormArray to match display order
            unitsArray.clear({ emitEvent: false });
            this._cachedUnits.forEach(control => {
                unitsArray.push(control, { emitEvent: false });
            });
            
            this._lastUnitsVersion = currentVersion;
        }
        return this._cachedUnits;
    }

    // Filtered and paginated lists
    get filteredTalkgroups(): FormGroup[] {
        let filtered = this.talkgroups;
        if (this.talkgroupsSearchTerm.trim()) {
            const search = this.talkgroupsSearchTerm.toLowerCase();
            filtered = filtered.filter(tg => {
                const label = (tg.value.label || '').toLowerCase();
                const name = (tg.value.name || '').toLowerCase();
                const id = (tg.value.talkgroupRef || '').toString();
                return label.includes(search) || name.includes(search) || id.includes(search);
            });
        }
        return filtered;
    }

    get paginatedTalkgroups(): FormGroup[] {
        const start = this.talkgroupsPage * this.talkgroupsPageSize;
        const end = start + this.talkgroupsPageSize;
        return this.filteredTalkgroups.slice(start, end);
    }

    get filteredUnits(): FormGroup[] {
        let filtered = this.units;
        if (this.unitsSearchTerm.trim()) {
            const search = this.unitsSearchTerm.toLowerCase();
            filtered = filtered.filter(unit => {
                const label = (unit.value.label || '').toLowerCase();
                const id = (unit.value.unitRef || '').toString();
                return label.includes(search) || id.includes(search);
            });
        }
        return filtered;
    }

    get paginatedUnits(): FormGroup[] {
        const start = this.unitsPage * this.unitsPageSize;
        const end = start + this.unitsPageSize;
        return this.filteredUnits.slice(start, end);
    }

    get filteredSites(): FormGroup[] {
        let filtered = this.sites;
        if (this.sitesSearchTerm.trim()) {
            const search = this.sitesSearchTerm.toLowerCase();
            filtered = filtered.filter(site => {
                const label = (site.value.label || '').toLowerCase();
                return label.includes(search);
            });
        }
        return filtered;
    }

    get paginatedSites(): FormGroup[] {
        const start = this.sitesPage * this.sitesPageSize;
        const end = start + this.sitesPageSize;
        return this.filteredSites.slice(start, end);
    }

    get hasSelectedTalkgroups(): boolean {
        return this.selectedTalkgroupIndices.size > 0;
    }

    get allTalkgroupsSelected(): boolean {
        return this.talkgroups.length > 0 && this.selectedTalkgroupIndices.size === this.talkgroups.length;
    }

    @ViewChildren(MatExpansionPanel) private panels: QueryList<MatExpansionPanel> | undefined;

    constructor(private adminService: RdioScannerAdminService) {
    }

    addSite(): void {
        const sites = this.form.get('sites') as FormArray | null;

        sites?.insert(0, this.adminService.newSiteForm());

        this.form.markAsDirty();
        this._lastSitesVersion++;
        this.sitesPage = 0; // Reset to first page
    }

    addTalkgroup(): void {
        const talkgroups = this.form.get('talkgroups') as FormArray | null;

        talkgroups?.insert(0, this.adminService.newTalkgroupForm());

        this.form.markAsDirty();
        this._lastTalkgroupsVersion++;
        this.talkgroupsPage = 0; // Reset to first page
    }

    addUnit(): void {
        const units = this.form.get('units') as FormArray | null;

        units?.insert(0, this.adminService.newUnitForm());

        this.form.markAsDirty();
        this._lastUnitsVersion++;
        this.unitsPage = 0; // Reset to first page
    }

    blacklistTalkgroup(index: number): void {
        const talkgroup = this.talkgroups[index];

        const id = talkgroup.value.id;

        if (typeof id !== 'number') {
            return;
        }

        const blacklists = this.form.get('blacklists') as FormControl | null;

        blacklists?.setValue(blacklists.value?.trim() ? `${blacklists.value},${id}` : `${id}`);

        this.removeTalkgroup(index);
    }

    closeAll(): void {
        this.panels?.forEach((panel) => panel.close());
    }

    drop(event: CdkDragDrop<FormGroup[]>): void {
        if (event.previousIndex !== event.currentIndex) {
            // Get the actual FormArray (not the sorted view)
            const talkgroupsArray = this.form.get('talkgroups') as FormArray | null;
            
            if (!talkgroupsArray) {
                return;
            }

            // Move items in the visual sorted array
            moveItemInArray(event.container.data, event.previousIndex, event.currentIndex);

            // Update order values to match new positions
            event.container.data.forEach((dat, idx) => dat.get('order')?.setValue(idx + 1, { emitEvent: false }));

            // CRITICAL: Reorder the actual FormArray to match the new sorted order
            // This ensures getRawValue() returns talkgroups in the correct order
            const reorderedControls = event.container.data.slice();
            talkgroupsArray.clear({ emitEvent: false });
            reorderedControls.forEach(control => {
                talkgroupsArray.push(control, { emitEvent: false });
            });

            this.form.markAsDirty();
            this._lastTalkgroupsVersion++;
        }
    }

    removeSite(index: number): void {
        const sites = this.form.get('sites') as FormArray | null;

        sites?.removeAt(index);

        sites?.markAsDirty();
        this._lastSitesVersion++;
    }

    removeTalkgroup(index: number): void {
        const talkgroups = this.form.get('talkgroups') as FormArray | null;

        talkgroups?.removeAt(index);

        talkgroups?.markAsDirty();
        this._lastTalkgroupsVersion++;
    }

    removeUnit(index: number): void {
        const units = this.form.get('units') as FormArray | null;

        units?.removeAt(index);

        units?.markAsDirty();
        this._lastUnitsVersion++;
    }

    // Bulk selection methods
    toggleTalkgroupSelection(index: number): void {
        if (this.selectedTalkgroupIndices.has(index)) {
            this.selectedTalkgroupIndices.delete(index);
        } else {
            this.selectedTalkgroupIndices.add(index);
        }
    }

    isTalkgroupSelected(index: number): boolean {
        return this.selectedTalkgroupIndices.has(index);
    }

    selectAllTalkgroups(): void {
        this.selectedTalkgroupIndices.clear();
        this.talkgroups.forEach((_, index) => {
            this.selectedTalkgroupIndices.add(index);
        });
    }

    unselectAllTalkgroups(): void {
        this.selectedTalkgroupIndices.clear();
    }

    bulkAssignGroup(): void {
        if (this.bulkAssignGroupId === null || this.selectedTalkgroupIndices.size === 0) {
            return;
        }

        this.selectedTalkgroupIndices.forEach(index => {
            const talkgroup = this.talkgroups[index];
            const groupIds = talkgroup.get('groupIds')?.value || [];
            
            // Add the group if it's not already assigned
            if (!groupIds.includes(this.bulkAssignGroupId)) {
                const newGroupIds = [...groupIds, this.bulkAssignGroupId];
                talkgroup.get('groupIds')?.setValue(newGroupIds);
                talkgroup.markAsDirty();
            }
        });

        this.form.markAsDirty();
        this.unselectAllTalkgroups();
        this.bulkAssignGroupId = null;
    }

    bulkAssignTag(): void {
        if (this.bulkAssignTagId === null || this.selectedTalkgroupIndices.size === 0) {
            return;
        }

        this.selectedTalkgroupIndices.forEach(index => {
            const talkgroup = this.talkgroups[index];
            talkgroup.get('tagId')?.setValue(this.bulkAssignTagId);
            talkgroup.markAsDirty();
        });

        this.form.markAsDirty();
        this.unselectAllTalkgroups();
        this.bulkAssignTagId = null;
    }

    bulkRemoveGroup(): void {
        if (this.bulkAssignGroupId === null || this.selectedTalkgroupIndices.size === 0) {
            return;
        }

        this.selectedTalkgroupIndices.forEach(index => {
            const talkgroup = this.talkgroups[index];
            const groupIds = talkgroup.get('groupIds')?.value || [];
            
            // Remove the group if it's assigned
            const newGroupIds = groupIds.filter((id: number) => id !== this.bulkAssignGroupId);
            talkgroup.get('groupIds')?.setValue(newGroupIds);
            talkgroup.markAsDirty();
        });

        this.form.markAsDirty();
        this.unselectAllTalkgroups();
        this.bulkAssignGroupId = null;
    }

    sortTalkgroupsAlphabetically(): void {
        const talkgroupsArray = this.form.get('talkgroups') as FormArray | null;
        
        if (!talkgroupsArray || talkgroupsArray.length === 0) {
            return;
        }

        // Get all talkgroup controls and sort them alphabetically by label
        const sortedControls = talkgroupsArray.controls.slice().sort((a, b) => {
            const labelA = (a.get('label')?.value || '').toString().trim().toLowerCase();
            const labelB = (b.get('label')?.value || '').toString().trim().toLowerCase();
            return labelA.localeCompare(labelB);
        });

        // Update order values based on new alphabetical positions
        sortedControls.forEach((control, idx) => {
            control.get('order')?.setValue(idx + 1, { emitEvent: false });
        });

        // Rebuild the FormArray to match the new sorted order
        talkgroupsArray.clear({ emitEvent: false });
        sortedControls.forEach(control => {
            talkgroupsArray.push(control, { emitEvent: false });
        });

        this.form.markAsDirty();
        this.unselectAllTalkgroups();
        this._lastTalkgroupsVersion++;
    }

    getTalkgroupsErrorSummary(): string {
        const talkgroupsArray = this.form.get('talkgroups') as FormArray | null;
        if (!talkgroupsArray) return 'Invalid talkgroups';
        
        const invalidCount = talkgroupsArray.controls.filter(c => c.invalid).length;
        return `${invalidCount} invalid talkgroup${invalidCount !== 1 ? 's' : ''}`;
    }

    getTalkgroupErrors(talkgroupForm: FormGroup): string {
        const errors: string[] = [];
        
        if (talkgroupForm.get('talkgroupRef')?.hasError('required')) {
            errors.push('ID required');
        } else if (talkgroupForm.get('talkgroupRef')?.hasError('duplicate')) {
            errors.push('Duplicate ID');
        } else if (talkgroupForm.get('talkgroupRef')?.hasError('min')) {
            errors.push('Invalid ID');
        }
        
        if (talkgroupForm.get('label')?.hasError('required')) {
            errors.push('Label required');
        }
        
        if (talkgroupForm.get('name')?.hasError('required')) {
            errors.push('Name required');
        }

        if (talkgroupForm.get('groupIds')?.hasError('required')) {
            errors.push('Group required');
        }

        if (talkgroupForm.get('tagId')?.hasError('required')) {
            errors.push('Tag required');
        }

        return errors.join(', ');
    }

    // Pagination methods
    onTalkgroupsSearchChange(searchTerm: string): void {
        this.talkgroupsSearchTerm = searchTerm;
        this.talkgroupsPage = 0; // Reset to first page on search
    }

    onTalkgroupsPageChange(page: number): void {
        this.talkgroupsPage = page;
    }

    get talkgroupsTotalPages(): number {
        return Math.ceil(this.filteredTalkgroups.length / this.talkgroupsPageSize);
    }

    onUnitsSearchChange(searchTerm: string): void {
        this.unitsSearchTerm = searchTerm;
        this.unitsPage = 0;
    }

    onUnitsPageChange(page: number): void {
        this.unitsPage = page;
    }

    get unitsTotalPages(): number {
        return Math.ceil(this.filteredUnits.length / this.unitsPageSize);
    }

    onSitesSearchChange(searchTerm: string): void {
        this.sitesSearchTerm = searchTerm;
        this.sitesPage = 0;
    }

    onSitesPageChange(page: number): void {
        this.sitesPage = page;
    }

    get sitesTotalPages(): number {
        return Math.ceil(this.filteredSites.length / this.sitesPageSize);
    }
}
