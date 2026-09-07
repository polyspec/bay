# Generates the artboards of the source-list design. They share one chrome, so
# it is written once here and each artboard supplies its selection and detail.
#
# The palette is baked into each file rather than switched at render time: a
# handlebars hole in `class` did not resolve, and every artboard came out blank.
# Only the token block below differs between the two appearances - which is also
# how the real window behaves, since AppKit resolves the same system colors.

PRELUDE = '''
    body { margin: 0; }
'''

DARK_TOKENS = '''
    .win {
      --bg: #2D3135; --titlebar: #313336; --sidebar: rgba(0,0,0,0.16);
      --card: #1E1E1E; --border: #3A3A3A; --hairline: #303030;
      --label: #DDDDDD; --secondary: #8E8E8E; --tertiary: #6E6E6E;
      --muted: #7A7A7A; --link: #419CFF; --link-hover: #6FB4FF;
      --accent: #0A84FF; --green: #32D74B; --amber: #FF9F0A;
      --off: rgba(255,255,255,0.24); --ring: rgba(255,255,255,0.12);
      --btn: #3B3E40; --btn-border: #565656; --btn-label: #DDDDDD;
      --btn-quiet-border: #4A4A4A; --btn-quiet-label: #B4B4B4;
      --btn-dim: #2E3033; --btn-dim-label: #5C5C5C; --btn-dim-border: #444444;
      --banner-bg: #24211A; --banner-border: #4A3D1E;
      --banner-label: #E9DFC2; --banner-text: #C9BE9E;
      --halo: rgba(50,215,75,0.16); --halo-warn: rgba(255,159,10,0.16);
    }
'''

LIGHT_TOKENS = '''
    .win {
      --bg: #EEF1F3; --titlebar: #F2F6F8; --sidebar: rgba(0,0,0,0.035);
      --card: #FFFFFF; --border: #C8CBCD; --hairline: #E6E8EA;
      --label: #1A1A1A; --secondary: #808080; --tertiary: #9A9A9A;
      --muted: #7C7C7C; --link: #0068DA; --link-hover: #0052AE;
      --accent: #007AFF; --green: #34C759; --amber: #FF9500;
      --off: rgba(0,0,0,0.2); --ring: rgba(0,0,0,0.10);
      --btn: #FFFFFF; --btn-border: #C8CBCD; --btn-label: #1A1A1A;
      --btn-quiet-border: #D2D5D7; --btn-quiet-label: #4A4A4A;
      --btn-dim: #F3F4F5; --btn-dim-label: #B0B0B0; --btn-dim-border: #DCDEE0;
      --banner-bg: #FFF6E0; --banner-border: #EBD9A8;
      --banner-label: #6B4E11; --banner-text: #7C6329;
      --halo: rgba(52,199,89,0.20); --halo-warn: rgba(255,149,0,0.20);
    }
'''

BASE = '''
    a { color: var(--link); text-decoration: none; }
    a:hover { color: var(--link-hover); text-decoration: underline; }
    .win { width: 900px; height: 700px; box-sizing: border-box;
      background: var(--bg); color: var(--label);
      font-family: -apple-system, "SF Pro Text", system-ui, sans-serif;
      display: flex; flex-direction: column; overflow: hidden; border-radius: 10px; }
    .titlebar { height: 52px; flex: none; background: var(--titlebar);
      border-bottom: 1px solid var(--border); display: flex; align-items: center;
      gap: 8px; padding: 0 20px; }
    .light-btn { width: 12px; height: 12px; border-radius: 50%; }
    .title { flex: 1; text-align: center; font-size: 13px; font-weight: 600; margin-left: -60px; }
    .grow { flex: 1; }
    .split { flex: 1; display: flex; overflow: hidden; }

    /* Source list: navigation on the left, one subject on the right. */
    .side { width: 216px; flex: none; background: var(--sidebar);
      border-right: 1px solid var(--border); padding: 10px 0;
      display: flex; flex-direction: column; gap: 2px; }
    .sgroup { font-size: 11px; font-weight: 600; color: var(--tertiary);
      padding: 12px 16px 4px; letter-spacing: 0.02em; }
    .sitem { display: flex; align-items: center; gap: 9px; margin: 0 8px;
      padding: 6px 8px; border-radius: 6px; font-size: 13px; }
    .sitem.sel { background: var(--accent); color: #FFFFFF; }
    .sitem .count { margin-left: auto; font-size: 11px; color: var(--muted);
      font-variant-numeric: tabular-nums; }
    .sitem.sel .count { color: rgba(255,255,255,0.75); }
    .sitem.sel .dot { box-shadow: inset 0 0 0 1px rgba(255,255,255,0.4); }

    .dot { width: 9px; height: 9px; border-radius: 50%; flex: none;
      box-shadow: inset 0 0 0 1px var(--ring); }
    .on { background: var(--green); } .warn { background: var(--amber); }
    .off { background: var(--off); }

    .detail { flex: 1; display: flex; flex-direction: column; overflow: hidden; }
    .dhead { padding: 18px 24px 14px; border-bottom: 1px solid var(--border);
      display: flex; align-items: center; gap: 12px; }
    .dhead h1 { margin: 0; font-size: 20px; font-weight: 600; letter-spacing: -0.01em; }
    .dhead .sub { font-size: 12px; color: var(--muted); }
    .dbody { flex: 1; padding: 18px 24px 22px; display: flex; flex-direction: column;
      gap: 18px; overflow: hidden; }

    .card { background: var(--card); border: 1px solid var(--border);
      border-radius: 10px; overflow: hidden; }
    .clabel { display: flex; align-items: baseline; gap: 8px;
      font-size: 11px; font-weight: 600; color: var(--secondary);
      padding: 0 4px 7px; letter-spacing: 0.02em; }
    .clabel .note { font-weight: 400; letter-spacing: 0; color: var(--tertiary); }
    .row { display: flex; align-items: center; gap: 12px; padding: 10px 12px 10px 14px; }
    .row + .row { border-top: 1px solid var(--hairline); }
    .name { width: 96px; flex: none; font-size: 13px; }
    .addr { flex: 1; font-size: 12px; }
    .addr.muted { color: var(--muted); }
    .meta { font-size: 12px; color: var(--muted); font-variant-numeric: tabular-nums; }
    .btn { font-size: 12px; padding: 3px 10px; border-radius: 6px;
      border: 1px solid var(--btn-border); background: var(--btn);
      color: var(--btn-label); flex: none; }
    .btn.quiet { background: transparent; border-color: var(--btn-quiet-border);
      color: var(--btn-quiet-label); }
    .btn.hero { background: var(--accent); border-color: var(--accent);
      color: #FFFFFF; font-weight: 500; }
    .btn.dim { background: var(--btn-dim); color: var(--btn-dim-label);
      border-color: var(--btn-dim-border); }
    .kv { display: flex; align-items: center; padding: 9px 14px; font-size: 12px; }
    .kv + .kv { border-top: 1px solid var(--hairline); }
    .kv b { width: 116px; flex: none; font-weight: 400; color: var(--secondary); }
    .chip { font-size: 11px; color: var(--secondary); border: 1px solid var(--border);
      border-radius: 5px; padding: 1px 6px; }
    .faint { color: var(--tertiary); }
    .strong { color: var(--label); }

    /* The dashboard exists because a source list otherwise hides everything
       that is not selected. */
    .verdict { display: flex; align-items: center; gap: 10px; }
    .beacon { width: 11px; height: 11px; border-radius: 50%; background: var(--green);
      box-shadow: 0 0 0 4px var(--halo); }
    .beacon.warn { background: var(--amber); box-shadow: 0 0 0 4px var(--halo-warn); }
    .headline { font-size: 15px; font-weight: 600; letter-spacing: -0.01em; }
    .subline { font-size: 12px; color: var(--muted); margin-top: 2px; }
    .banner { background: var(--banner-bg); border: 1px solid var(--banner-border);
      border-radius: 10px; padding: 14px 16px; display: flex;
      align-items: center; gap: 14px; }
    .banner .txt { font-size: 12px; color: var(--banner-text); line-height: 1.5; }
    .banner .txt b { display: block; color: var(--banner-label); font-size: 13px;
      font-weight: 600; margin-bottom: 2px; }
    .banner .txt em { font-style: normal; color: var(--banner-label); }
    .pdots { display: flex; gap: 4px; }
'''

SIDEBAR = [
  ("machine", "MACHINE", [("Dashboard", "dash", "on", ""),
                          ("Domains", "domains", "on", "2"),
                          ("Certificates", "certs", "on", "6")]),
  ("proj", "PROJECTS", [("platform", "platform", "on", "3/3"),
                        ("guidecheck", "guidecheck", "on", "2/2"),
                        ("soksak", "soksak", "off", "0/4")]),
]

def sidebar(selected, state=None, projects=None):
    out = []
    for key, group, items in SIDEBAR:
        if key == "proj" and projects is not None:
            items = projects
        out.append('      <div class="sgroup">%s</div>' % group)
        for label, k, dot, count in items:
            if state and k in state:
                dot = state[k]
            sel = " sel" if k == selected else ""
            c = '<span class="count">%s</span>' % count if count else ""
            out.append('      <div class="sitem%s"><span class="dot %s"></span>%s%s</div>'
                       % (sel, dot, label, c))
    return "\n".join(out)

TEMPLATE = '''<!doctype html>
<html>
<head>
  <meta charset="utf-8">
  <script src="./support.js"></script>
</head>
<body>
<x-dc>
<helmet>
  <style>{css}</style>
</helmet>

<div class="win">
  <div class="titlebar">
    <div class="light-btn" style="background:#FF5F57"></div>
    <div class="light-btn" style="background:#FEBC2E"></div>
    <div class="light-btn" style="background:#28C840"></div>
    <div class="title">bay</div>
  </div>

  <div class="split">
    <div class="side">
{sidebar}
    </div>

    <div class="detail">
{detail}
    </div>
  </div>
</div>
</x-dc>
</body>
</html>
'''

def write(path, selected, detail, state=None, projects=None, theme="dark"):
    tokens = LIGHT_TOKENS if theme == "light" else DARK_TOKENS
    open(path, "w").write(TEMPLATE.format(
        css=PRELUDE + tokens + BASE,
        sidebar=sidebar(selected, state, projects),
        detail=detail.rstrip() + "\n"))
    print("wrote", path, theme)
