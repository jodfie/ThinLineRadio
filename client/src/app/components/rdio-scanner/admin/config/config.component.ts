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

import { ChangeDetectionStrategy, ChangeDetectorRef, Component, OnDestroy, OnInit, QueryList, ViewChild, ViewChildren, ViewEncapsulation } from '@angular/core';
import { FormArray, FormControl, FormGroup } from '@angular/forms';
import { MatExpansionPanel } from '@angular/material/expansion';
import { AdminEvent, RdioScannerAdminService, Config } from '../admin.service';
import { RdioScannerAdminUsersComponent } from './users/users.component';
import { RdioScannerAdminUserGroupsComponent } from './user-groups/user-groups.component';

@Component({
    changeDetection: ChangeDetectionStrategy.OnPush,
    encapsulation: ViewEncapsulation.None,
    selector: 'rdio-scanner-admin-config',
    styleUrls: ['./config.component.scss'],
    templateUrl: './config.component.html',
})
export class RdioScannerAdminConfigComponent implements OnDestroy, OnInit {
    docker = false;

    form: FormGroup | undefined;
    
    private isImportedForReview = false;

    get apikeys(): FormArray {
        return (this.form?.get('apikeys') as FormArray) || new FormArray([]);
    }

    get dirwatch(): FormArray {
        return (this.form?.get('dirwatch') as FormArray) || new FormArray([]);
    }

    get downstreams(): FormArray {
        return (this.form?.get('downstreams') as FormArray) || new FormArray([]);
    }

    get groups(): FormArray {
        return (this.form?.get('groups') as FormArray) || new FormArray([]);
    }

    get options(): FormGroup {
        return (this.form?.get('options') as FormGroup) || new FormGroup({});
    }

    get systems(): FormArray {
        return (this.form?.get('systems') as FormArray) || new FormArray([]);
    }

    get tags(): FormArray {
        return (this.form?.get('tags') as FormArray) || new FormArray([]);
    }

    get users(): FormArray {
        return (this.form?.get('users') as FormArray) || new FormArray([]);
    }

    get userGroups(): FormArray {
        return (this.form?.get('userGroups') as FormArray) || new FormArray([]);
    }

    get keywordLists(): FormArray {
        return (this.form?.get('keywordLists') as FormArray) || new FormArray([]);
    }

    private config: Config | undefined;

    private eventSubscription;

    @ViewChildren(MatExpansionPanel) private panels: QueryList<MatExpansionPanel> | undefined;
    @ViewChild(RdioScannerAdminUsersComponent) private usersComponent: RdioScannerAdminUsersComponent | undefined;
    @ViewChild(RdioScannerAdminUserGroupsComponent) private userGroupsComponent: RdioScannerAdminUserGroupsComponent | undefined;

    constructor(
        private adminService: RdioScannerAdminService,
        private ngChangeDetectorRef: ChangeDetectorRef,
    ) {
        this.eventSubscription = this.adminService.event.subscribe(async (event: AdminEvent) => {
            if ('authenticated' in event && event.authenticated === true) {
                this.config = await this.adminService.getConfig();

                this.reset();
            }

            if ('config' in event) {
                this.config = event.config;

                if (this.form?.pristine) {
                    this.reset();
                }
            }

            if ('docker' in event) {
                this.docker = event.docker ?? false;
            }
        });
    }

    ngOnDestroy(): void {
        this.eventSubscription.unsubscribe();
    }

    async ngOnInit(): Promise<void> {
        // Only load data if user is authenticated
        if (this.adminService.authenticated) {
            await this.adminService.loadAlerts();

            this.config = await this.adminService.getConfig();

            this.reset();
        }
    }

    closeAll(): void {
        this.panels?.forEach((panel) => panel.close());
    }

    reset(config = this.config, options?: { dirty?: boolean, isImport?: boolean }): void {
        this.form = this.adminService.newConfigForm(config);
        
        // Track if this reset is from an "Import for Review"
        this.isImportedForReview = options?.isImport === true;

        this.form.statusChanges.subscribe(() => {
            this.ngChangeDetectorRef.markForCheck();
        });

        this.groups.valueChanges.subscribe(() => {
            this.systems.controls.forEach((system) => {
                const talkgroups = system.get('talkgroups') as FormArray;

                talkgroups.controls.forEach((talkgroup) => {
                    const groupIds = talkgroup.get('groupIds') as FormArray;

                    groupIds.updateValueAndValidity({ onlySelf: true });

                    if (groupIds.errors) {
                        groupIds.markAsTouched({ onlySelf: true });
                    }
                });
            });
        });

        this.tags.valueChanges.subscribe(() => {
            this.systems.controls.forEach((system) => {
                const talkgroups = system.get('talkgroups') as FormArray;

                talkgroups.controls.forEach((talkgroup) => {
                    const tagId = talkgroup.get('tagId') as FormControl;

                    tagId.updateValueAndValidity({ onlySelf: true });

                    if (tagId.errors) {
                        tagId.markAsTouched({ onlySelf: true });
                    }
                });
            });
        });

        if (options?.dirty === true) {
            this.form.markAsDirty();
        }

        this.ngChangeDetectorRef.markForCheck();

        // Reload users and user groups components if they exist
        // Use setTimeout to ensure components are initialized
        setTimeout(() => {
            if (this.usersComponent) {
                this.usersComponent.loadUsers();
            }
            if (this.userGroupsComponent) {
                this.userGroupsComponent.loadGroups();
            }
        }, 0);
    }

    async save(): Promise<void> {
        this.form?.markAsPristine();

        const formValue = this.form?.getRawValue();
        
        // If NOT imported for review, exclude users, userGroups, and keywordLists (managed via dedicated endpoints)
        // If imported for review, include them and do a full import
        const isFullImport = this.isImportedForReview;
        
        if (!isFullImport) {
            delete formValue.users;
            delete formValue.userGroups;
            delete formValue.keywordLists;
        }
        
        // Clear the import flag after save
        this.isImportedForReview = false;

        // Convert tone sets from flat form structure to nested structure
        if (formValue?.systems) {
            formValue.systems = formValue.systems.map((system: any) => {
                if (system.talkgroups) {
                    system.talkgroups = system.talkgroups.map((talkgroup: any) => {
                        if (talkgroup.toneSets && Array.isArray(talkgroup.toneSets)) {
                            talkgroup.toneSets = talkgroup.toneSets.map((toneSet: any) => {
                                const converted: any = {
                                    id: toneSet.id,
                                    label: toneSet.label,
                                    tolerance: toneSet.tolerance || 10,
                                };
                                
                                if (toneSet.minDuration) {
                                    converted.minDuration = toneSet.minDuration;
                                }
                                
                                if (toneSet.aToneFrequency || toneSet.aToneMinDuration) {
                                    converted.aTone = {
                                        frequency: toneSet.aToneFrequency,
                                        minDuration: toneSet.aToneMinDuration || 0,
                                    };
                                    if (toneSet.aToneMaxDuration) {
                                        converted.aTone.maxDuration = toneSet.aToneMaxDuration;
                                    }
                                }
                                
                                if (toneSet.bToneFrequency || toneSet.bToneMinDuration) {
                                    converted.bTone = {
                                        frequency: toneSet.bToneFrequency,
                                        minDuration: toneSet.bToneMinDuration || 0,
                                    };
                                    if (toneSet.bToneMaxDuration) {
                                        converted.bTone.maxDuration = toneSet.bToneMaxDuration;
                                    }
                                }
                                
                                if (toneSet.longToneFrequency || toneSet.longToneMinDuration) {
                                    converted.longTone = {
                                        frequency: toneSet.longToneFrequency,
                                        minDuration: toneSet.longToneMinDuration || 0,
                                    };
                                    if (toneSet.longToneMaxDuration) {
                                        converted.longTone.maxDuration = toneSet.longToneMaxDuration;
                                    }
                                }
                                
                                return converted;
                            });
                        }
                        return talkgroup;
                    });
                }
                return system;
            });
        }

        // Convert transcription config from form structure
        if (formValue?.options) {
            if (formValue.options.transcriptionEnabled !== undefined) {
                formValue.options.transcriptionConfig = formValue.options.transcriptionConfig || {};
                formValue.options.transcriptionConfig.enabled = formValue.options.transcriptionEnabled;
            }
            // Remove transcriptionEnabled if it exists separately (we use transcriptionConfig.enabled)
            if (formValue.options.transcriptionEnabled !== undefined && formValue.options.transcriptionConfig) {
                formValue.options.transcriptionConfig.enabled = formValue.options.transcriptionEnabled;
            }
            
            // Convert hallucination patterns from textarea string to array
            if (formValue.options.transcriptionConfig?.hallucinationPatterns) {
                const patternsString = formValue.options.transcriptionConfig.hallucinationPatterns;
                if (typeof patternsString === 'string') {
                    // Split by newlines, trim each line, and filter out empty lines
                    formValue.options.transcriptionConfig.hallucinationPatterns = patternsString
                        .split('\n')
                        .map((line: string) => line.trim())
                        .filter((line: string) => line.length > 0);
                }
            }
            
            // Convert AssemblyAI word boost from textarea string to array
            if (formValue.options.transcriptionConfig?.assemblyAIWordBoost) {
                const wordBoostString = formValue.options.transcriptionConfig.assemblyAIWordBoost;
                if (typeof wordBoostString === 'string') {
                    // Split by newlines, trim each line, and filter out empty lines
                    formValue.options.transcriptionConfig.assemblyAIWordBoost = wordBoostString
                        .split('\n')
                        .map((line: string) => line.trim())
                        .filter((line: string) => line.length > 0);
                }
            }
            
            // Always use hardcoded relay server URL
            formValue.options.relayServerURL = 'https://tlradioserver.thinlineds.com';
        }

        await this.adminService.saveConfig(formValue, isFullImport);
    }
}
