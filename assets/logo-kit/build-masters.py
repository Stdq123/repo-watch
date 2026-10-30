from pathlib import Path
from fontTools.ttLib import TTFont
from fontTools.pens.svgPathPen import SVGPathPen
from fontTools.pens.transformPen import TransformPen
from xml.sax.saxutils import escape

out = Path('assets/logo-kit')
fontpath = Path.home() / 'AppData/Roaming/Python/Python313/site-packages/matplotlib/mpl-data/fonts/ttf/DejaVuSans-Bold.ttf'
font = TTFont(fontpath)
glyphs = font.getGlyphSet()
cmap = font.getBestCmap()
upm = font['head'].unitsPerEm
hmtx = font['hmtx'].metrics
kern = {}
if 'kern' in font:
    for table in font['kern'].kernTables:
        if table.version == 0: kern.update(table.kernTable)
forest = '#163c2b'
lime = '#c5ee73'
cream = '#dff4cd'
w = 'M27 55h25l24 111 38-74h27l39 74 24-111h25l-36 147h-24l-41-78-40 78H64L27 55Z'
# Symbol uses the approved concept's original construction.
def symbol(color=forest, dot=None):
    return f'<path d="{w}" fill="{color}"/><circle cx="128" cy="60" r="13" fill="{dot or color}"/>'
def text_paths(text, x, baseline, size, color):
    scale = size / upm
    result = []
    previous = None
    cursor = x
    for char in text:
        name = cmap[ord(char)]
        if previous: cursor += kern.get((previous, name), 0) * scale
        pen = SVGPathPen(glyphs)
        tp = TransformPen(pen, (scale, 0, 0, -scale, cursor, baseline))
        glyphs[name].draw(tp)
        d = pen.getCommands()
        if d: result.append(f'<path d="{d}" fill="{color}"/>')
        cursor += hmtx[name][0] * scale
        previous = name
    return ''.join(result), cursor-x
def svg(view, title, body):
    return f'<svg xmlns="http://www.w3.org/2000/svg" viewBox="{view}" role="img" aria-label="{escape(title)}"><title>{escape(title)}</title>{body}</svg>\n'
(out/'symbol.svg').write_text(svg('0 0 256 256','Repo Watch symbol',symbol()),encoding='utf-8')
(out/'symbol-small.svg').write_text(svg('0 0 256 256','Repo Watch small symbol',symbol()),encoding='utf-8')
(out/'symbol-on-dark.svg').write_text(svg('0 0 256 256','Repo Watch symbol on dark',symbol(cream,lime)),encoding='utf-8')
(out/'app-icon-color.svg').write_text(svg('0 0 256 256','Repo Watch app icon',f'<rect x="8" y="8" width="240" height="240" rx="58" fill="{forest}"/><g transform="translate(38 38) scale(.703125)">{symbol(cream,lime)}</g>'),encoding='utf-8')
paths,width=text_paths('Repo Watch',0,0,90,forest)
textbody,width=text_paths('Repo Watch',300,160,90,forest)
canvas=300+width+24
(out/'horizontal.svg').write_text(svg(f'0 0 {canvas:.2f} 256','Repo Watch horizontal logo',symbol()+textbody),encoding='utf-8')
textwhite,_=text_paths('Repo Watch',300,160,90,cream)
(out/'horizontal-on-dark.svg').write_text(svg(f'0 0 {canvas:.2f} 256','Repo Watch horizontal logo on dark',symbol(cream,lime)+textwhite),encoding='utf-8')
stacksize=70
_,sw=text_paths('Repo Watch',0,0,stacksize,forest)
stackwidth=max(420,sw+48)
stacktext,_=text_paths('Repo Watch',(stackwidth-sw)/2,328,stacksize,forest)
(out/'stacked.svg').write_text(svg(f'0 0 {stackwidth:.2f} 352','Repo Watch stacked logo',f'<g transform="translate({(stackwidth-256)/2:.2f} 0)">{symbol()}</g>'+stacktext),encoding='utf-8')
word,width=text_paths('Repo Watch',12,95,100,forest)
(out/'wordmark.svg').write_text(svg(f'0 0 {width+24:.2f} 125','Repo Watch wordmark',word),encoding='utf-8')
print('Wordmark outlined from DejaVu Sans Bold:', fontpath)
