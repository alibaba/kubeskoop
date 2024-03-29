export function getErrorMessage(e: any): string {
    return e?.response?.data?.error || e.message || e.toString();
}

export function clamp(value: number, min: number, max: number): number {
    return Math.min(Math.max(value, min), max);
}
