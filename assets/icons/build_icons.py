"""Render the checked-in SVG icons to alpha masks for the native Windows UI.

Requires resvg-py==0.5.0 only when regenerating assets. The application itself
uses the checked-in PNG files and does not need Python or a network connection.
"""
from pathlib import Path

import resvg_py


ROOT = Path(__file__).resolve().parent


def main() -> None:
    for family in ("lucide", "simple-icons"):
        for svg_path in sorted((ROOT / family).glob("*.svg")):
            svg = svg_path.read_text(encoding="utf-8").replace("currentColor", "#000000")
            png = resvg_py.svg_to_bytes(svg_string=svg, width=256, height=256, skip_system_fonts=True)
            svg_path.with_suffix(".png").write_bytes(png)


if __name__ == "__main__":
    main()
