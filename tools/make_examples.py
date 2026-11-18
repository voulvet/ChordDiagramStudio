"""Creates the synthetic example files in example/ (fixed random seed, so the files are reproducible).

None of these numbers come from a real study. Values are simulated from a few latent factors
(cartilage damage, bone remodelling, synovial inflammation) for invented animals; Spearman r and
p-values are then calculated from the simulated values, so r and p are consistent with each other.

Usage: python3 tools/make_examples.py example/
Needs numpy, scipy and openpyxl.
"""
import sys, os
import numpy as np
from scipy.stats import spearmanr
from openpyxl import Workbook
from openpyxl.styles import Font

OUT = sys.argv[1] if len(sys.argv) > 1 else 'example'
os.makedirs(OUT, exist_ok=True)

# Category (as written before "_" in the variable names) -> full name, short ring label, ring group
CATEGORIES = [
    ('Macroscopy',   'Cartilage macroscopy',          'Macroscopy',     'Articular cartilage'),
    ('Histology',    'Cartilage histology',           'Histology',      'Articular cartilage'),
    ('Biomechanics', 'Cartilage biomechanics',        'Biomechanics',   'Articular cartilage'),
    ('CollagenII',   'Cartilage type-II collagen',    'Type-II collagen', 'Articular cartilage'),
    ('MicroCT',      'Subchondral bone microstructure', 'Microstructure', 'Subchondral bone'),
    ('Synovium',     'Synovial membrane histology',   'Histology',      'Synovium'),
    ('Biochemistry', 'Cartilage biochemistry',        'Biochemistry',   'Articular cartilage'),
]
FULL = {c: f for c, f, _, _ in CATEGORIES}

# (category, measurement, latent loadings (cartilage, bone, synovium), regional)
MEASURES = [
    ('Macroscopy', 'Surface score', (0.9, 0.1, 0.2), True),
    ('Macroscopy', 'India ink area', (0.8, 0.0, 0.2), True),
    ('Macroscopy', 'Osteophyte grade', (0.4, 0.6, 0.3), False),
    ('Histology', 'Structure score', (0.9, 0.2, 0.1), True),
    ('Histology', 'Proteoglycan loss', (0.8, 0.1, 0.2), True),
    ('Histology', 'Tidemark score', (0.5, 0.6, 0.0), True),
    ('Biomechanics', 'Instantaneous modulus', (-0.8, 0.1, 0.0), True),
    ('Biomechanics', 'Equilibrium modulus', (-0.7, 0.2, -0.1), True),
    ('Biomechanics', 'Permeability', (0.6, 0.0, 0.2), True),
    ('CollagenII', 'Type-II staining', (-0.7, 0.0, -0.2), True),
    ('MicroCT', 'BV/TV', (0.2, 0.9, 0.0), True),
    ('MicroCT', 'Trabecular thickness', (0.1, 0.8, 0.0), True),
    ('MicroCT', 'Plate thickness', (0.3, 0.8, 0.1), True),
    ('MicroCT', 'Bone mineral density', (0.0, 0.7, -0.1), True),
    ('Synovium', 'Synovitis score', (0.3, 0.0, 0.9), False),
    ('Synovium', 'Lining thickness', (0.2, 0.1, 0.8), False),
    ('Synovium', 'Inflammatory infiltrate', (0.2, 0.0, 0.7), False),
    ('Biochemistry', 'GAG content', (-0.6, 0.0, -0.2), True),
    ('Biochemistry', 'DNA content', (0.3, 0.0, 0.3), True),
]

# Each invented "study" samples different regions and a different subset of measurements.
STUDIES = {
    'studyA': dict(regions=['MED', 'LAT'], skip={'Tidemark score', 'DNA content'}, n=14),
    'studyB': dict(regions=['ANT', 'CEN', 'POST'], skip={'Permeability', 'Plate thickness', 'GAG content', 'Inflammatory infiltrate'}, n=12),
}
# Group differences: in the "OA" group cartilage, bone and synovium are coupled more strongly,
# and trabecular thickness changes its relation to cartilage damage (gives sign changes).
OA_LOADINGS = {'Trabecular thickness': (-0.7, 0.7, 0.0)}
GROUPS = {
    'control': np.array([[1.0, 0.30, 0.25], [0.30, 1.0, 0.10], [0.25, 0.10, 1.0]]),
    'oa':      np.array([[1.0, 0.55, 0.60], [0.55, 1.0, 0.30], [0.60, 0.30, 1.0]]),
}


def variables(study):
    cfg = STUDIES[study]
    out = []
    for cat, name, load, regional in MEASURES:
        if name in cfg['skip']:
            continue
        if regional:
            for r in cfg['regions']:
                out.append((f'{cat}_{name} ({r})', cat, r, np.array(load)))
        else:
            out.append((f'{cat}_{name}', cat, '', np.array(load)))
    return out


def simulate(study, group, seed):
    rng = np.random.default_rng(seed)
    cfg = STUDIES[study]
    V = variables(study)
    n = cfg['n']
    F = rng.multivariate_normal(np.zeros(3), GROUPS[group], size=n)       # latent factors per animal
    reg_eff = {r: rng.normal(0, 0.6, size=n) for r in cfg['regions']}     # region-specific variation
    X = []
    for name, cat, r, load in V:
        if group == 'oa':
            for m, l in OA_LOADINGS.items():
                if f'_{m}' in name:
                    load = np.array(l)
        noise = 0.65 if group == 'control' else 0.55
        x = F @ load + rng.normal(0, noise, size=n)
        if r:
            x = x + reg_eff[r] * 0.5
        X.append(x)
    X = np.array(X).T
    rho, p = spearmanr(X)
    return V, rho, p


def write(path, V, rho, p, varsheet=True, catsheet=True, header='variable'):
    wb = Workbook()
    labels = [v[0] if header == 'variable' else v[1] for v in V]
    for k, (title, M) in enumerate([('Spearman r', rho), ('p values', p)]):
        ws = wb.active if k == 0 else wb.create_sheet()
        ws.title = title
        ws.append([None] + labels)
        for i, lab in enumerate(labels):
            row = [lab]
            for j in range(len(labels)):
                row.append(None if i == j and k == 1 else round(float(M[i, j]), 4 if k == 0 else 6))
            ws.append(row)
    if varsheet:
        ws = wb.create_sheet('Variables')
        ws.append(['Variable', 'Category', 'Region'])
        for name, cat, r, _ in V:
            ws.append([name, FULL[cat], r or None])
        for c in ws[1]:
            c.font = Font(bold=True)
        ws.column_dimensions['A'].width = 44
        ws.column_dimensions['B'].width = 34
    if catsheet:
        ws = wb.create_sheet('Categories')
        ws.append(['Category', 'Label', 'Group'])
        for _, full, lab, grp in CATEGORIES:
            ws.append([full, lab, grp])
        for c in ws[1]:
            c.font = Font(bold=True)
        ws.column_dimensions['A'].width = 34
        ws.column_dimensions['B'].width = 20
        ws.column_dimensions['C'].width = 22
    wb.save(path)
    print('wrote', path, len(V), 'variables')


# 1. category names in row 1 / column A (one header per variable)
V, rho, p = simulate('studyA', 'oa', 11)
write(f'{OUT}/example_matrices.xlsx', V, rho, p, varsheet=False, catsheet=False, header='category')
# 2. variable names with region codes, no extra sheets
V, rho, p = simulate('studyB', 'oa', 12)
write(f'{OUT}/example_variables_regions.xlsx', V, rho, p, varsheet=False, catsheet=False)
# 3. two time-points of one study (simple comparison)
for k, (g, seed) in enumerate([('control', 21), ('oa', 22)], 1):
    V, rho, p = simulate('studyA', g, seed)
    write(f'{OUT}/example_timepoint_{k}.xlsx', V, rho, p, varsheet=False, catsheet=False)
# 4. two studies x two groups, with Variables and Categories sheets
for study, sseed in [('studyA', 30), ('studyB', 40)]:
    for g, gseed in [('control', 1), ('oa', 2)]:
        V, rho, p = simulate(study, g, sseed + gseed)
        write(f'{OUT}/example_{study}_{g}.xlsx', V, rho, p)
