import {useEffect, useRef, useState} from "react";

export function useCopyToClipboard(text: string | null | undefined): { copied: boolean; copy: () => void } {
    const [copied, setCopied] = useState(false);
    const copyTimer = useRef<number | null>(null);

    useEffect(() => {
        return () => {
            if (copyTimer.current !== null) {
                window.clearTimeout(copyTimer.current);
            }
        };
    }, []);

    const copy = () => {
        if (!text || !navigator.clipboard) {
            return;
        }
        navigator.clipboard.writeText(text).then(() => {
            setCopied(true);
            if (copyTimer.current !== null) {
                window.clearTimeout(copyTimer.current);
            }
            copyTimer.current = window.setTimeout(() => setCopied(false), 1500);
        }).catch(() => {});
    };

    return {copied, copy};
}
