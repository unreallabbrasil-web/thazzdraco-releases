#!/usr/bin/env python3
"""
Gera resource.syso (COFF amd64) com RT_ICON + RT_GROUP_ICON + RT_MANIFEST,
para o linker do Go embutir o icone do dragao e o manifesto (requireAdministrator).
Self-contained: nao depende de windres/rc/goversioninfo. Tecnica equivalente ao rsrc.
"""
import struct, sys

ICO = "icon.ico"
MANIFEST = "app.manifest"
OUT = "resource.syso"

RT_ICON, RT_GROUP_ICON, RT_MANIFEST, RT_VERSION = 3, 14, 24, 16
LANG = 0x0409

# ---- VERSIONINFO (RT_VERSION) ----------------------------------------------
# Metadados de versao/empresa/produto. Um .exe SEM isso tem a aba "Detalhes" em
# branco, o que e um sinal classico de software suspeito para a heuristica de
# antivirus. Preencher reduz o "score" de suspeita (binario com identidade).
VER_FILE = (4, 0, 0, 0)
VER_PROD = (4, 0, 0, 0)
VER_STRINGS = [
    ("CompanyName",      "ThazZDraco FPS Otimizacao"),
    ("FileDescription",  "ThazzDraco Optimizer - Otimizador de PC para jogos"),
    ("FileVersion",      "4.0.0.0"),
    ("InternalName",     "ThazzDraco"),
    ("LegalCopyright",   "(c) 2026 ThazZDraco FPS Otimizacao"),
    ("OriginalFilename", "ThazzDraco.exe"),
    ("ProductName",      "ThazzDraco Optimizer"),
    ("ProductVersion",   "4.0.0.0"),
]


def _ver_node(key, value, is_text, children):
    """Serializa um no VS_VERSIONINFO (recursivo). Alinhamento DWORD relativo ao
    inicio do no: header tem 6 bytes, entao o corpo alinha em (6+len)%4==0."""
    body = bytearray(key.encode("utf-16-le") + b"\x00\x00")
    while (6 + len(body)) % 4 != 0:
        body += b"\x00"
    if value:
        body += value
        while (6 + len(body)) % 4 != 0:
            body += b"\x00"
    for c in children:
        while (6 + len(body)) % 4 != 0:
            body += b"\x00"
        body += c
    wValueLength = (len(value) // 2 if is_text else len(value)) if value else 0
    wLength = 6 + len(body)
    return struct.pack("<HHH", wLength, wValueLength, 1 if is_text else 0) + bytes(body)


def build_version():
    msf = (VER_FILE[0] << 16) | VER_FILE[1]
    lsf = (VER_FILE[2] << 16) | VER_FILE[3]
    msp = (VER_PROD[0] << 16) | VER_PROD[1]
    lsp = (VER_PROD[2] << 16) | VER_PROD[3]
    # VS_FIXEDFILEINFO (13 DWORDs): assinatura, struct ver, file ver MS/LS,
    # prod ver MS/LS, flags mask, flags, OS(NT_WINDOWS32), type(APP), subtype, datas.
    ffi = struct.pack("<13I", 0xFEEF04BD, 0x00010000, msf, lsf, msp, lsp,
                      0x3F, 0, 0x00040004, 1, 0, 0, 0)
    strs = [_ver_node(k, v.encode("utf-16-le") + b"\x00\x00", True, []) for k, v in VER_STRINGS]
    table = _ver_node("040904b0", b"", True, strs)            # lang 0409 + cp 1200 (unicode)
    sfi = _ver_node("StringFileInfo", b"", True, [table])
    trans = _ver_node("Translation", struct.pack("<HH", 0x0409, 0x04B0), False, [])
    vfi = _ver_node("VarFileInfo", b"", True, [trans])
    return _ver_node("VS_VERSION_INFO", ffi, False, [sfi, vfi])

def align(n, a=8):
    return (n + a - 1) // a * a

# ---- le icon.ico ------------------------------------------------------------
ico = open(ICO, "rb").read()
_, itype, count = struct.unpack_from("<HHH", ico, 0)
icon_images, grp_entries = [], []
for i in range(count):
    bW, bH, bCC, bR, wP, wBC, dwBIR, dwOff = struct.unpack_from("<BBBBHHII", ico, 6 + i * 16)
    icon_images.append(ico[dwOff:dwOff + dwBIR])
    grp_entries.append((bW, bH, bCC, bR, wP, wBC, dwBIR))

# GRPICONDIR (RT_GROUP_ICON): header + GRPICONDIRENTRY[count] (wId = i+1)
grp = struct.pack("<HHH", 0, 1, count)
for i, (bW, bH, bCC, bR, wP, wBC, dwBIR) in enumerate(grp_entries):
    grp += struct.pack("<BBBBHHIH", bW, bH, bCC, bR, wP, wBC, dwBIR, i + 1)

manifest = open(MANIFEST, "rb").read()

# ---- arvore de recursos: {tipo: {nome: bytes}} (lang fixo) ------------------
tree = {}
tree[RT_ICON] = {i + 1: icon_images[i] for i in range(count)}
tree[RT_GROUP_ICON] = {1: grp}
tree[RT_MANIFEST] = {1: manifest}
tree[RT_VERSION] = {1: build_version()}

types = sorted(tree.keys())

# ---- layout das estruturas (offsets relativos a secao) ----------------------
DIR_HDR, DIR_ENT, DATA_ENT = 16, 8, 16

# nivel 1 (tipos), nivel 2 (nomes por tipo), nivel 3 (lang por nome)
n_l3 = sum(len(tree[t]) for t in types)       # um dir de lang por nome
n_leaves = n_l3                               # uma folha por (tipo,nome,lang)

off_root = 0
size_root = DIR_HDR + DIR_ENT * len(types)

# diretorios de nivel 2 (um por tipo)
off = off_root + size_root
l2_off = {}
for t in types:
    l2_off[t] = off
    off += DIR_HDR + DIR_ENT * len(tree[t])

# diretorios de nivel 3 (um por nome)
l3_off = {}
for t in types:
    for name in sorted(tree[t].keys()):
        l3_off[(t, name)] = off
        off += DIR_HDR + DIR_ENT * 1

# data entries
data_ent_off = {}
for t in types:
    for name in sorted(tree[t].keys()):
        data_ent_off[(t, name)] = off
        off += DATA_ENT

# dados crus (alinhados a 8)
off = align(off, 8)
raw_off = {}
for t in types:
    for name in sorted(tree[t].keys()):
        raw_off[(t, name)] = off
        off += align(len(tree[t][name]), 8)

section_size = off

# ---- serializa a secao ------------------------------------------------------
sec = bytearray(section_size)

def put_dir(pos, entries):
    # IMAGE_RESOURCE_DIRECTORY: Characteristics, TimeDateStamp, Maj, Min,
    # NumberOfNamedEntries(0), NumberOfIdEntries
    struct.pack_into("<IIHHHH", sec, pos, 0, 0, 0, 0, 0, len(entries))
    p = pos + DIR_HDR
    for (idv, child_off, is_dir) in entries:
        val = child_off | (0x80000000 if is_dir else 0)
        struct.pack_into("<II", sec, p, idv, val)
        p += DIR_ENT

# root -> aponta para diretorios de nivel 2
put_dir(off_root, [(t, l2_off[t], True) for t in types])
# nivel 2 -> aponta para diretorios de nivel 3
for t in types:
    put_dir(l2_off[t], [(name, l3_off[(t, name)], True) for name in sorted(tree[t].keys())])
# nivel 3 -> aponta para data entry (folha, sem high bit)
for t in types:
    for name in sorted(tree[t].keys()):
        put_dir(l3_off[(t, name)], [(LANG, data_ent_off[(t, name)], False)])

# data entries + dados crus
relocs = []  # offsets (na secao) do campo OffsetToData de cada data entry
for t in types:
    for name in sorted(tree[t].keys()):
        de = data_ent_off[(t, name)]
        ro = raw_off[(t, name)]
        blob = tree[t][name]
        struct.pack_into("<IIII", sec, de, ro, len(blob), 0, 0)  # OffsetToData(reloc), Size, CodePage, Reserved
        relocs.append(de)  # o campo OffsetToData esta no inicio do data entry
        sec[ro:ro + len(blob)] = blob

# ---- monta o COFF -----------------------------------------------------------
IMAGE_FILE_MACHINE_AMD64 = 0x8664
IMAGE_REL_AMD64_ADDR32NB = 0x0003

num_relocs = len(relocs)
ptr_raw = 20 + 40                       # apos header COFF + 1 section header
ptr_relocs = ptr_raw + section_size
ptr_symtab = ptr_relocs + 10 * num_relocs
num_symbols = 2                         # simbolo da secao + aux

# COFF header
coff = struct.pack("<HHIIIHH",
    IMAGE_FILE_MACHINE_AMD64, 1, 0, ptr_symtab, num_symbols, 0, 0)

# section header (.rsrc)
name = b".rsrc\0\0\0"
sec_hdr = name + struct.pack("<IIIIIIHHI",
    0, 0, section_size, ptr_raw, ptr_relocs, 0, num_relocs, 0, 0x40000040)

# relocations (cada 10 bytes): VirtualAddress, SymbolTableIndex(0), Type
reloc_bytes = b"".join(struct.pack("<IIH", va, 0, IMAGE_REL_AMD64_ADDR32NB) for va in relocs)

# symbol table: simbolo ".rsrc" (idx0) + aux (idx1)
sym = name + struct.pack("<IhHBB", 0, 1, 0, 3, 1)  # Value,SectionNumber,Type,Storage(STATIC),NumAux
aux = struct.pack("<IHHIHBxxx", section_size, num_relocs, 0, 0, 0, 0)

# String table do COFF: obrigatoria mesmo vazia (4 bytes = seu proprio tamanho).
string_table = struct.pack("<I", 4)

out = coff + sec_hdr + bytes(sec) + reloc_bytes + sym + aux + string_table
open(OUT, "wb").write(out)
print(f"{OUT}: {len(out)} bytes | {count} icones, manifesto {len(manifest)}b, {num_relocs} relocs")
