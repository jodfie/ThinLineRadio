/*
 * Shared types for transcript parser admin UI and Options JSON.
 */

export interface FuzzyWord {
    word: string;
    maxDistance: number;
    aliases?: string[];
    reject?: string[];
}

export interface ChannelShorthand {
    label: string;
    dispatch: string;
    separator?: string;
}

export interface TranscriptConfig {
    unitTypes: FuzzyWord[];
    unitPrefixes: FuzzyWord[];
    dispatchNames: FuzzyWord[];
    channelSeparators: FuzzyWord[];
    channelShorthands: ChannelShorthand[];
    corrections: FuzzyWord[];
}

export type FuzzyListKey = 'unitTypes' | 'unitPrefixes' | 'dispatchNames' | 'channelSeparators' | 'corrections';
