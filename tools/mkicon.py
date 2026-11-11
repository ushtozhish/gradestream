# Рисует иконку GradeStream и собирает .ico + COFF-ресурс (.syso) для Go.
import struct, io, sys
from PIL import Image, ImageDraw

def draw(sz):
    S = 512
    im = Image.new('RGBA', (S, S), (0, 0, 0, 0))
    d = ImageDraw.Draw(im)
    d.rounded_rectangle([16, 16, S - 16, S - 16], radius=110, fill=(22, 22, 24, 255))
    # три перекрывающихся круга — RGB, символ цветокоррекции
    cols = [(232, 72, 64, 215), (80, 200, 120, 215), (70, 130, 240, 215)]
    import math
    for i, c in enumerate(cols):
        a = -math.pi / 2 + i * 2 * math.pi / 3
        cx, cy, r = 256 + 70 * math.cos(a), 270 + 70 * math.sin(a), 120
        layer = Image.new('RGBA', (S, S), (0, 0, 0, 0))
        ImageDraw.Draw(layer).ellipse([cx - r, cy - r, cx + r, cy + r], fill=c)
        im = Image.alpha_composite(im, layer)
    d = ImageDraw.Draw(im)
    # «эфир» — оранжевая точка
    d.ellipse([370, 60, 450, 140], fill=(224, 160, 64, 255), outline=(22, 22, 24, 255), width=10)
    return im.resize((sz, sz), Image.LANCZOS)

sizes = [16, 24, 32, 48, 64, 128, 256]
imgs = []
for s in sizes:
    b = io.BytesIO(); draw(s).save(b, 'PNG'); imgs.append(b.getvalue())

# --- .ico
ico = struct.pack('<HHH', 0, 1, len(sizes))
off = 6 + 16 * len(sizes)
for s, data in zip(sizes, imgs):
    ico += struct.pack('<BBBBHHII', s % 256, s % 256, 0, 0, 1, 32, len(data), off)
    off += len(data)
ico += b''.join(imgs)
open(sys.argv[1], 'wb').write(ico)

# --- .syso (COFF, x64) с RT_ICON + RT_GROUP_ICON
def dirhdr(n_id): return struct.pack('<IIHHHH', 0, 0, 0, 0, 0, n_id)
n = len(sizes)
# Раскладка: корень(2 типа) -> [RT_ICON dir (n ids)] -> lang dirs ; [RT_GROUP_ICON dir (1 id)] -> lang dir ; data entries ; данные
blobs = list(imgs)
grp = struct.pack('<HHH', 0, 1, n) + b''.join(
    struct.pack('<BBBBHHIH', s % 256, s % 256, 0, 0, 1, 32, len(d), i + 1) for i, (s, d) in enumerate(zip(sizes, imgs)))
blobs.append(grp)

root = 0
root_sz = 16 + 8 * 2
icon_dir = root_sz
icon_dir_sz = 16 + 8 * n
grp_dir = icon_dir + icon_dir_sz
grp_dir_sz = 16 + 8
lang_dirs = grp_dir + grp_dir_sz
lang_sz = 16 + 8
data_entries = lang_dirs + lang_sz * (n + 1)
data_start = data_entries + 16 * (n + 1)

out = bytearray()
out += dirhdr(2) + struct.pack('<II', 3, 0x80000000 | icon_dir) + struct.pack('<II', 14, 0x80000000 | grp_dir)
out += dirhdr(n) + b''.join(struct.pack('<II', i + 1, 0x80000000 | (lang_dirs + lang_sz * i)) for i in range(n))
out += dirhdr(1) + struct.pack('<II', 1, 0x80000000 | (lang_dirs + lang_sz * n))
for i in range(n + 1):
    out += dirhdr(1) + struct.pack('<II', 0x0409, data_entries + 16 * i)
assert len(out) == data_entries
pos = data_start
relocs = []
offs = []
for b in blobs:
    pos = (pos + 7) & ~7
    offs.append(pos); pos += len(b)
for i, b in enumerate(blobs):
    relocs.append(len(out))           # поле OffsetToData — относительный адрес, нужна релокация
    out += struct.pack('<IIII', offs[i], len(b), 0, 0)
for i, b in enumerate(blobs):
    out += b'\0' * (offs[i] - len(out)); out += b
while len(out) % 8: out += b'\0'

# COFF
nsec = 1
hdr_sz = 20 + 40
raw_ptr = hdr_sz
reloc_ptr = raw_ptr + len(out)
sym_ptr = reloc_ptr + 10 * len(relocs)
coff = struct.pack('<HHIIIHH', 0x8664, nsec, 0, sym_ptr, 1, 0, 0)
sec = struct.pack('<8sIIIIIIHHI', b'.rsrc', 0, 0, len(out), raw_ptr, reloc_ptr, 0, len(relocs), 0, 0xC0000040)
rel = b''.join(struct.pack('<IIH', r, 0, 3) for r in relocs)   # IMAGE_REL_AMD64_ADDR32NB к символу 0 (.rsrc)
sym = struct.pack('<8sIhHBB', b'.rsrc', 0, 1, 0, 3, 0)          # static section symbol
strtab = struct.pack('<I', 4)
open(sys.argv[2], 'wb').write(coff + sec + out + rel + sym + strtab)
print('ok', len(ico), 'bytes ico')
