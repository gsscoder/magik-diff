import {useEffect, useRef, useState} from "react";
import {Quit, WindowIsMaximised, WindowMinimise, WindowToggleMaximise} from "../wailsjs/runtime/runtime";

function MinimiseIcon() {
    return (
        <svg width="10" height="10" viewBox="0 0 10 10" fill="none" aria-hidden="true">
            <path d="M1 5H9" stroke="currentColor" />
        </svg>
    );
}

function MaximiseIcon() {
    return (
        <svg width="10" height="10" viewBox="0 0 10 10" fill="none" aria-hidden="true">
            <rect x="1.5" y="1.5" width="7" height="7" stroke="currentColor" />
        </svg>
    );
}

function RestoreIcon() {
    return (
        <svg width="10" height="10" viewBox="0 0 10 10" fill="none" aria-hidden="true">
            <path d="M3 1H9V7" stroke="currentColor" />
            <rect x="1" y="3" width="6" height="6" stroke="currentColor" />
        </svg>
    );
}

function CloseIcon() {
    return (
        <svg width="10" height="10" viewBox="0 0 10 10" fill="none" aria-hidden="true">
            <path d="M1.5 1.5L8.5 8.5M8.5 1.5L1.5 8.5" stroke="currentColor" />
        </svg>
    );
}

type TitleBarProps = {
    onOpenDirectory: () => void;
};

declare global {
    interface Window {
        wails?: {flags: {enableResize: boolean; resizeEdge?: string}};
    }
}

function TitleBar({onOpenDirectory}: TitleBarProps) {
    const [maximised, setMaximised] = useState(false);
    const [menuOpen, setMenuOpen] = useState(false);
    const resizeFrame = useRef(0);
    const menuRef = useRef<HTMLDivElement>(null);

    useEffect(() => {
        function poll() {
            WindowIsMaximised().then(setMaximised);
        }
        poll();
        // Wails v2.13 emits no window-state-changed event, so Aero-snap and
        // Win+Up-arrow maximise/restore are only observable via the resize
        // event they trigger; coalesce with rAF since resize fires rapidly
        // during a live drag-resize.
        function onResize() {
            cancelAnimationFrame(resizeFrame.current);
            resizeFrame.current = requestAnimationFrame(poll);
        }
        window.addEventListener("resize", onResize);
        return () => {
            window.removeEventListener("resize", onResize);
            cancelAnimationFrame(resizeFrame.current);
        };
    }, []);

    useEffect(() => {
        // Frameless Wails claims a 6px band inside every window edge as a
        // resize handle and swallows the mousedown, which kills clicks on the
        // window buttons and shrinks the drag strip. A maximised window can't
        // be resized anyway, so drop the band there.
        const flags = window.wails?.flags;
        if (!flags) {
            return;
        }
        flags.enableResize = !maximised;
        if (maximised) {
            flags.resizeEdge = undefined;
            document.documentElement.style.cursor = "";
        }
    }, [maximised]);

    useEffect(() => {
        if (!menuOpen) {
            return;
        }
        function onClickOutside(e: MouseEvent) {
            if (menuRef.current && !menuRef.current.contains(e.target as Node)) {
                setMenuOpen(false);
            }
        }
        function onKeyDown(e: KeyboardEvent) {
            if (e.key === "Escape") {
                setMenuOpen(false);
            }
        }
        document.addEventListener("mousedown", onClickOutside);
        document.addEventListener("keydown", onKeyDown);
        return () => {
            document.removeEventListener("mousedown", onClickOutside);
            document.removeEventListener("keydown", onKeyDown);
        };
    }, [menuOpen]);

    return (
        <div className="titlebar">
            <div className="titlebar-menu" ref={menuRef}>
                <button
                    type="button"
                    className="titlebar-menu-button"
                    aria-label="Repo menu"
                    aria-haspopup="true"
                    aria-expanded={menuOpen}
                    onClick={() => setMenuOpen((open) => !open)}
                >
                    Repo
                </button>
                {menuOpen && (
                    <div className="titlebar-menu-dropdown">
                        <button
                            type="button"
                            className="titlebar-menu-item"
                            onClick={() => {
                                setMenuOpen(false);
                                onOpenDirectory();
                            }}
                        >
                            Open directory...
                        </button>
                        <div className="titlebar-menu-separator" />
                        <button
                            type="button"
                            className="titlebar-menu-item"
                            onClick={() => {
                                setMenuOpen(false);
                                Quit();
                            }}
                        >
                            Exit
                        </button>
                    </div>
                )}
            </div>
            <div className="titlebar-drag" onDoubleClick={() => WindowToggleMaximise()}>
                <span className="titlebar-spacer" />
            </div>
            <div className="titlebar-controls">
                <button
                    type="button"
                    className="titlebar-button"
                    aria-label="Minimise"
                    title="Minimise"
                    onClick={() => WindowMinimise()}
                >
                    <MinimiseIcon />
                </button>
                <button
                    type="button"
                    className="titlebar-button"
                    aria-label={maximised ? "Restore" : "Maximise"}
                    title={maximised ? "Restore" : "Maximise"}
                    onClick={() => WindowToggleMaximise()}
                >
                    {maximised ? <RestoreIcon /> : <MaximiseIcon />}
                </button>
                <button
                    type="button"
                    className="titlebar-button titlebar-close"
                    aria-label="Close"
                    title="Close"
                    onClick={() => Quit()}
                >
                    <CloseIcon />
                </button>
            </div>
        </div>
    );
}

export default TitleBar;
