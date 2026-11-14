#!/usr/bin/env python3
"""Generate original local vector avatars for catalog personas."""
from pathlib import Path
import sys

target = Path(sys.argv[1]) / 'apps/api/uploads/catalog/avatar'
target.mkdir(parents=True, exist_ok=True)
palettes = [('c7ded5', '587f72', '263c38'), ('e4d5f3', '9583bb', '453452'),
            ('f6d3bc', 'ce8c6d', '533c32'), ('cbdff4', '759fca', '2b4661'),
            ('f5e4b4', 'bd9b50', '493f28'), ('eed0d9', 'c1869d', '513745')]
for index in range(24):
    background, shirt, hair = palettes[index % len(palettes)]
    shift = (index % 3 - 1) * 3
    if index % 4 == 0:
        subject = f'<path d="M24 80Q20 40 32 24L48 36L64 24Q77 40 72 80Z" fill="#{shirt}"/><ellipse cx="48" cy="53" rx="22" ry="21" fill="#{background}"/><path d="M35 51h5m16 0h5M45 61l3 3 3-3" fill="none" stroke="#{hair}" stroke-width="3" stroke-linecap="round"/>'
    else:
        subject = f'<path d="M14 100Q18 66 48 66Q78 66 82 100Z" fill="#{shirt}"/><path d="M25 50Q17 18 48 15Q78 15 72 56L67 78H30Z" fill="#{hair}"/><ellipse cx="48" cy="47" rx="20" ry="24" fill="#f5d7bd"/><path d="M27 40Q27 15 51 18Q73 19 70 43Q54 37 {42+shift} 25Q39 38 27 40Z" fill="#{hair}"/><path d="M38 48h2m16 0h2M43 60q5 4 10 0" fill="none" stroke="#{hair}" stroke-width="2.5" stroke-linecap="round"/>'
    svg = f'<svg xmlns="http://www.w3.org/2000/svg" width="96" height="96" viewBox="0 0 96 96"><defs><clipPath id="c"><circle cx="48" cy="48" r="48"/></clipPath></defs><g clip-path="url(#c)"><path fill="#{background}" d="M0 0h96v96H0z"/><circle cx="78" cy="16" r="28" fill="white" opacity=".2"/>{subject}</g></svg>'
    (target / f'{index:02d}.svg').write_text(svg, encoding='utf-8')
print(f'Created 24 local avatars in {target}')
