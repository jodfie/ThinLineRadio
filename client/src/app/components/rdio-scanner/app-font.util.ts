/*
 * Shared app font list + apply helper.
 * User-selected fonts apply only to the public scanner UI (.scanner-shell),
 * not the admin console.
 */

export interface AppFontOption {
    name: string;
    value: string;
    displayName: string;
}

export const APP_FONTS: AppFontOption[] = [
    { name: 'Inter', value: 'Inter, ui-sans-serif, system-ui, sans-serif', displayName: 'Inter (Default)' },
    { name: 'SpaceGrotesk', value: '"Space Grotesk", ui-sans-serif, sans-serif', displayName: 'Space Grotesk (Display)' },
    { name: 'Roboto', value: 'Roboto, sans-serif', displayName: 'Roboto' },
    { name: 'Rajdhani', value: 'Rajdhani, sans-serif', displayName: 'Rajdhani' },
    { name: 'JetBrainsMono', value: '"JetBrains Mono", ui-monospace, monospace', displayName: 'JetBrains Mono' },
];

const SCANNER_FONT_SELECTOR = '.scanner-shell';

function syncFontVars(el: HTMLElement, value: string): void {
    el.style.setProperty('--tlr-font-primary', value);
    el.style.setProperty('--tlr-font-numeric', value);
}

function clearScannerFont(el: HTMLElement): void {
    el.style.removeProperty('--tlr-font-primary');
    el.style.removeProperty('--tlr-font-numeric');
    el.style.removeProperty('font-family');
    el.style.removeProperty('font-size');
    delete el.dataset['appFont'];
}

export function applyAppFont(fontName: string): void {
    const font = APP_FONTS.find(f => f.name === fontName) ?? APP_FONTS[0];

    document.querySelectorAll(SCANNER_FONT_SELECTOR).forEach((el) => {
        if (el instanceof HTMLElement) {
            clearScannerFont(el);
        }
    });

    document.querySelectorAll(SCANNER_FONT_SELECTOR).forEach((el) => {
        if (!(el instanceof HTMLElement)) {
            return;
        }
        el.dataset['appFont'] = font.name;
        syncFontVars(el, font.value);
        el.style.fontFamily = font.value;
    });

    document.documentElement.dataset['appFont'] = font.name;
}
