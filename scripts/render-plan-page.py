#!/usr/bin/env python3
"""Render a docs/plan-*.md design record as a standalone styled HTML page.

The Markdown file is the source of record. This script is a one-way renderer:
never hand-edit the generated HTML, because the next run overwrites it.

    python3 scripts/render-plan-page.py docs/plan-effort-leveling.md -o /tmp/plan.html

Dependency-free by design (stdlib only) — the repo has no pandoc and no Markdown
package, and a docs page is not worth adding one for. That means it supports the
Markdown subset the plan records actually use, and nothing else:

    headings (#, ##, ###)      pipe tables with an alignment row
    paragraphs                 - and 1. lists, incl. hanging continuation lines
    > blockquotes              > [!NOTE] / [!IMPORTANT] / [!WARNING] alerts
    --- rules                  `code`, **bold**, *italic*, [links](url)

Four conventions in the source get a visual treatment, so the page reads as a
designed document rather than a Markdown dump:

1. A leading `**Key**: value` run after the H1 becomes the masthead meta strip.
2. A `## Table of Contents` list becomes the sticky side nav and is dropped from
   the body (headings get GitHub-style slug ids, so its anchors resolve).
3. ✅ ⚠️ 📋 ❌ 🚧 at the start of a list item or table cell become status pills.
4. A table cell that is exactly a control key (A, B1, B2, C, D, V, X, or a pair
   like `A × B1`) is set in the accent mono face, and a purely numeric cell gets
   tabular figures.

Anything outside the subset is passed through escaped rather than guessed at, so
an unsupported construct shows up as visible plain text instead of silently
vanishing.
"""

import argparse
import html
import pathlib
import re
import sys

STATUS = {
    "✅": ("ok", "verified"),
    "⚠️": ("warn", "caveat"),
    "❌": ("no", "rejected"),
    "📋": ("plan", "planned"),
    "🚧": ("warn", "in progress"),
}

ALERTS = {
    "NOTE": ("", "Note"),
    "TIP": ("", "Tip"),
    "IMPORTANT": ("", "Important"),
    "WARNING": (" hard", "Warning"),
    "CAUTION": (" hard", "Caution"),
}

CONTROL_KEY = re.compile(r"^(?:[ABCDVX]\d?|Bounds)(?:\s*×\s*(?:[ABCDVX]\d?|all))?$")
NUMERIC_CELL = re.compile(r"^[\d\s.,/%+×−-]+$")

CSS = """
:root {
  --ground:#EBEEF1; --surface:#FFFFFF; --sunk:#E2E7EC; --ink:#141E28;
  --muted:#59697A; --rule:#CFD7DF; --accent:#8F5406; --accent-soft:#F0E4D2;
  --pay:#14603C; --fail:#8B2E29;
  --display:"Archivo","Helvetica Neue",Arial,sans-serif;
  --body:"Source Serif 4",Georgia,"Times New Roman",serif;
  --mono:"IBM Plex Mono",ui-monospace,"SF Mono",Menlo,monospace;
}
@media (prefers-color-scheme: dark) {
  :root:not([data-theme="light"]) {
    --ground:#0E151D; --surface:#161F29; --sunk:#1B2632; --ink:#E3E9EF;
    --muted:#94A4B3; --rule:#2B3845; --accent:#E4A857; --accent-soft:#2E2517;
    --pay:#55C08C; --fail:#E4817A;
  }
}
:root[data-theme="dark"] {
  --ground:#0E151D; --surface:#161F29; --sunk:#1B2632; --ink:#E3E9EF;
  --muted:#94A4B3; --rule:#2B3845; --accent:#E4A857; --accent-soft:#2E2517;
  --pay:#55C08C; --fail:#E4817A;
}
*{box-sizing:border-box}
body{background:var(--ground);color:var(--ink);font-family:var(--body);
  font-size:1.0625rem;line-height:1.6;-webkit-text-size-adjust:100%}

.masthead{border-bottom:1px solid var(--rule);background:var(--surface)}
.masthead-in{max-width:78rem;margin:0 auto;padding:2.75rem 1rem 2rem}
.eyebrow{font-family:var(--mono);font-size:.6875rem;font-weight:500;
  letter-spacing:.14em;text-transform:uppercase;color:var(--muted);margin:0 0 1rem}
h1{font-family:var(--display);font-weight:700;font-size:clamp(2rem,6vw,3.1rem);
  line-height:1.04;letter-spacing:-.021em;text-wrap:balance;margin:0 0 1.5rem}
.meta{display:flex;flex-wrap:wrap;gap:1rem 2.25rem;font-family:var(--mono);
  font-size:.75rem;color:var(--muted);border-top:1px solid var(--rule);padding-top:1.25rem}
.meta div{max-width:34rem}
.meta b{display:block;font-weight:500;font-size:.625rem;letter-spacing:.12em;
  text-transform:uppercase;opacity:.72;margin-bottom:.2rem}
.meta span{color:var(--ink)}

.shell{max-width:78rem;margin:0 auto;padding:0 1rem 5rem}
@media (min-width:62rem){
  .shell{display:grid;grid-template-columns:13.5rem minmax(0,1fr);gap:3.5rem;align-items:start}
}
.index{padding-block:2.25rem;font-family:var(--mono);font-size:.75rem;line-height:1.5}
@media (min-width:62rem){
  .index{position:sticky;top:env(safe-area-inset-top,0px);max-height:100vh;overflow-y:auto}
}
.index h2{font-size:.625rem;font-weight:600;letter-spacing:.14em;text-transform:uppercase;
  color:var(--muted);margin:0 0 .85rem;padding-bottom:.6rem;border-bottom:1px solid var(--rule)}
.index ol{list-style:none;margin:0;padding:0;display:grid;gap:.45rem;counter-reset:idx}
.index li{counter-increment:idx;display:flex;gap:.55rem;margin:0}
.index li::before{content:counter(idx,decimal-leading-zero);color:var(--muted);opacity:.6;flex:none}
.index a{color:var(--ink);text-decoration:none;border-bottom:1px solid transparent}
.index a:hover{color:var(--accent);border-bottom-color:var(--accent)}

main{padding-block:2.25rem;min-width:0}
section{margin-bottom:3.5rem;scroll-margin-top:1.5rem}
section>*{max-width:68ch}
section>.wide{max-width:none}
h2{font-family:var(--display);font-weight:600;font-size:clamp(1.3rem,3vw,1.6rem);
  letter-spacing:-.012em;line-height:1.18;text-wrap:balance;margin:0 0 1rem;
  padding-top:1.1rem;border-top:2px solid var(--ink)}
h3{font-family:var(--display);font-weight:600;font-size:1.0625rem;
  letter-spacing:-.004em;margin:2.25rem 0 .6rem}
p{margin:0 0 1rem;text-wrap:pretty}
a{color:var(--accent);text-underline-offset:.18em}
strong{font-weight:600}
em{font-style:italic}
code{font-family:var(--mono);font-size:.8em;background:var(--sunk);
  border:1px solid var(--rule);border-radius:2px;padding:.08em .32em}
p code,li code,td code{white-space:nowrap}
hr{border:0;border-top:1px solid var(--rule);margin:2.5rem 0}

ul,ol{margin:0 0 1rem;padding-left:1.5rem}
li{margin-bottom:.5rem}
li::marker{color:var(--muted);font-family:var(--mono);font-size:.85em}
ol>li{padding-left:.15rem}

.ctl,.key{font-family:var(--mono);font-size:.78em;font-weight:600;
  background:var(--accent-soft);color:var(--accent);border-radius:2px;
  padding:.1em .38em;white-space:nowrap}
.st{font-family:var(--mono);font-size:.625rem;font-weight:600;letter-spacing:.07em;
  text-transform:uppercase;padding:.18em .45em;border-radius:2px;
  border:1px solid currentColor;white-space:nowrap;margin-right:.4em}
.st.ok{color:var(--pay)} .st.no{color:var(--fail)}
.st.plan{color:var(--muted)} .st.warn{color:var(--accent)}

.note{border-left:3px solid var(--accent);background:var(--surface);
  padding:1.15rem 1.25rem;margin:0 0 1.5rem}
.note>:last-child{margin-bottom:0}
.note .label{font-family:var(--mono);font-size:.625rem;font-weight:600;
  letter-spacing:.13em;text-transform:uppercase;color:var(--accent);margin:0 0 .5rem}
.note.hard{border-left-color:var(--fail)}
.note.hard .label{color:var(--fail)}

blockquote{margin:1.75rem 0;padding:0 0 0 1.25rem;border-left:2px solid var(--rule);
  font-family:var(--display);font-weight:500;font-size:clamp(1.1rem,2.6vw,1.35rem);
  line-height:1.32;letter-spacing:-.008em;text-wrap:balance}
blockquote p{margin:0}

.wide{overflow-x:auto;margin:0 0 1.5rem;border:1px solid var(--rule);background:var(--surface)}
table{border-collapse:collapse;width:100%;font-family:var(--display);
  font-size:.8125rem;font-variant-numeric:tabular-nums}
th,td{text-align:left;padding:.6rem 1rem;border-bottom:1px solid var(--rule);vertical-align:top}
thead th{font-weight:600;font-size:.6875rem;letter-spacing:.06em;text-transform:uppercase;
  color:var(--muted);white-space:nowrap}
tbody tr:last-child td{border-bottom:0}
td.num{font-family:var(--mono);white-space:nowrap}

footer{border-top:1px solid var(--rule);background:var(--surface)}
.footer-in{max-width:78rem;margin:0 auto;padding:2rem 1rem 2.5rem;
  font-family:var(--mono);font-size:.75rem;color:var(--muted)}
.footer-in p{max-width:70ch;margin:0 0 .6rem}
a:focus-visible,.index a:focus-visible{outline:2px solid var(--accent);outline-offset:2px}
"""


def slug(text):
    """GitHub's heading-slug algorithm, so the source's own anchors resolve.

    Note the single `\\s` rather than `\\s+`: GitHub replaces each space with a
    hyphen instead of collapsing runs, so a removed em dash leaves a double
    hyphen ("Phase 0 — the replay" -> "phase-0--the-replay"). Collapsing here
    would silently break every cross-link written against GitHub's rendering.
    """
    s = re.sub(r"`|\*|\[|\]|\(.*?\)", "", text).strip().lower()
    s = re.sub(r"[^a-z0-9 -]", "", s)
    return re.sub(r"\s", "-", s)


def status_pill(text):
    """Pull a leading status emoji off `text`, returning (pill_html, rest)."""
    stripped = text.lstrip()
    for emoji, (cls, label) in STATUS.items():
        if stripped.startswith(emoji):
            rest = stripped[len(emoji):].lstrip()
            # A bolded restatement right after the emoji becomes the pill label.
            m = re.match(r"\*\*(.{1,28}?)\.?\*\*[ .]*", rest)
            if m:
                label = html.escape(m.group(1).lower())
                return f'<span class="st {cls}">{label}</span>', rest[m.end():]
            return f'<span class="st {cls}">{label}</span>', rest
    return "", text


def link(m):
    """Render a Markdown link.

    In-page anchors and absolute URLs stay navigable. A relative link to a repo
    file (`reference/workflow-leveling.md`) would 404 once the page is published
    away from the repo, so it is de-linked to its text plus the path — the reader
    still learns where the file is, and the page carries no dead link.
    """
    text, href = m.group(1), m.group(2)
    if href.startswith(("#", "http://", "https://", "mailto:")):
        return f'<a href="{html.escape(href, quote=True)}">{text}</a>'
    if href in text:
        return f"<strong>{text}</strong>"
    return f"<strong>{text}</strong> <code>{html.escape(href)}</code>"


def inline(text):
    out = html.escape(text, quote=False)
    # Code first, so markup inside backticks is not re-interpreted.
    spans = []

    def stash(m):
        spans.append(m.group(1))
        return f"\x00{len(spans) - 1}\x00"

    out = re.sub(r"`([^`]+)`", stash, out)
    out = re.sub(r"\[([^\]]+)\]\(([^)]+)\)", link, out)
    out = re.sub(r"\*\*([^*]+)\*\*", r"<strong>\1</strong>", out)
    out = re.sub(r"(?<![\w*])\*([^*\n]+)\*(?![\w*])", r"<em>\1</em>", out)
    out = re.sub(r"\x00(\d+)\x00", lambda m: f"<code>{spans[int(m.group(1))]}</code>", out)
    return out


def cell(text, header=False):
    pill, rest = status_pill(text)
    body = pill + inline(rest)
    if header:
        return f"<th>{body}</th>"
    # Control keys are usually bolded in the source; compare on the bare token.
    bare = rest.strip().strip("*`").strip()
    if CONTROL_KEY.match(bare):
        return f'<td><span class="key">{html.escape(bare)}</span></td>'
    cls = ' class="num"' if bare and NUMERIC_CELL.match(bare) else ""
    return f"<td{cls}>{body}</td>"


class Renderer:
    def __init__(self, lines):
        self.lines = lines
        self.i = 0
        self.out = []
        self.toc = []
        self.title = ""
        self.meta = []
        self.open_section = False

    def peek(self, offset=0):
        j = self.i + offset
        return self.lines[j] if j < len(self.lines) else None

    # -- block handlers -------------------------------------------------

    def table(self):
        rows = []
        while (line := self.peek()) is not None and line.strip().startswith("|"):
            rows.append([c.strip() for c in line.strip().strip("|").split("|")])
            self.i += 1
        if len(rows) < 2:
            return
        head, body = rows[0], rows[2:]  # rows[1] is the alignment row
        self.out.append('<div class="wide"><table>')
        self.out.append("<thead><tr>" + "".join(cell(c, True) for c in head) + "</tr></thead>")
        self.out.append("<tbody>")
        for row in body:
            self.out.append("<tr>" + "".join(cell(c) for c in row) + "</tr>")
        self.out.append("</tbody></table></div>")

    def listing(self, ordered):
        tag = "ol" if ordered else "ul"
        marker = re.compile(r"^\s*\d+\.\s+" if ordered else r"^\s*[-*]\s+")
        items = []
        while (line := self.peek()) is not None:
            if marker.match(line):
                items.append(marker.sub("", line).rstrip())
                self.i += 1
            elif line.strip() and line.startswith((" ", "\t")) and items:
                items[-1] += " " + line.strip()  # hanging continuation
                self.i += 1
            else:
                break
        self.out.append(f"<{tag}>")
        for item in items:
            pill, rest = status_pill(item)
            self.out.append(f"<li>{pill}{inline(rest)}</li>")
        self.out.append(f"</{tag}>")

    def quote(self):
        first = self.peek().lstrip("> ").rstrip()
        alert = re.match(r"^\[!(\w+)\]$", first)
        body = []
        self.i += 1 if alert else 0
        while (line := self.peek()) is not None and line.startswith(">"):
            body.append(re.sub(r"^>\s?", "", line).rstrip())
            self.i += 1
        if alert and alert.group(1).upper() in ALERTS:
            cls, label = ALERTS[alert.group(1).upper()]
            self.out.append(f'<div class="note{cls}"><p class="label">{label}</p>')
            for para in re.split(r"\n\s*\n", "\n".join(body).strip()):
                if para.strip():
                    self.out.append(f"<p>{inline(para.strip())}</p>")
            self.out.append("</div>")
        else:
            text = " ".join(x for x in body if x.strip())
            self.out.append(f"<blockquote><p>{inline(text)}</p></blockquote>")

    def paragraph(self):
        buf = []
        while (line := self.peek()) is not None and line.strip() \
                and not re.match(r"^(#{1,6} |\||>|---|\s*[-*] |\s*\d+\. )", line):
            buf.append(line.strip())
            self.i += 1
        if buf:
            self.out.append(f"<p>{inline(' '.join(buf))}</p>")

    def masthead_meta(self):
        """The `**Key**: value` run directly under the H1.

        A value may wrap over several source lines; a non-blank line that does
        not open a new key continues the one before it.
        """
        pairs = []
        while (line := self.peek()) is not None:
            stripped = line.strip()
            m = re.match(r"^\*\*(.+?)\*\*:\s*(.+)$", stripped)
            if m:
                pairs.append([m.group(1), m.group(2)])
                self.i += 1
            elif not stripped:
                self.i += 1
                if pairs and (nxt := self.peek()) is not None and not nxt.strip().startswith("**"):
                    break  # blank line then non-key text: the meta run is over
            elif pairs and not stripped.startswith("#"):
                pairs[-1][1] += " " + stripped
                self.i += 1
            else:
                break
        for key, value in pairs:
            pill, rest = status_pill(value)
            self.meta.append(
                f"<div><b>{html.escape(key)}</b>"
                f"<span>{pill}{inline(rest)}</span></div>"
            )

    def toc_section(self):
        """Consume a Table of Contents list into the side nav."""
        while (line := self.peek()) is not None and not line.startswith("## "):
            m = re.match(r"^\s*[-*]\s+\[([^\]]+)\]\(#([^)]+)\)", line)
            if m:
                self.toc.append((m.group(1), m.group(2)))
            self.i += 1

    # -- driver ---------------------------------------------------------

    def section(self, heading):
        if self.open_section:
            self.out.append("</section>")
        pill, rest = status_pill(heading)
        self.out.append(f'<section id="{slug(heading)}">')
        self.out.append(f"<h2>{pill}{inline(rest)}</h2>")
        self.open_section = True

    def run(self):
        while self.i < len(self.lines):
            line = self.peek()
            before = self.i
            if not line.strip():
                self.i += 1
            elif line.startswith("# ") and not self.title:
                self.title = line[2:].strip()
                self.i += 1
                self.masthead_meta()
            elif line.startswith("# "):
                # The plan records use a further H1 per phase; treat each as a
                # section rather than a second page title.
                self.i += 1
                self.section(line[2:].strip())
            elif line.startswith("## "):
                heading = line[3:].strip()
                self.i += 1
                if heading.lower() == "table of contents":
                    self.toc_section()
                    continue
                self.section(heading)
            elif line.startswith("### "):
                pill, rest = status_pill(line[4:].strip())
                self.out.append(f'<h3 id="{slug(line[4:])}">{pill}{inline(rest)}</h3>')
                self.i += 1
            elif line.strip().startswith("|"):
                self.table()
            elif line.startswith(">"):
                self.quote()
            elif re.match(r"^---+\s*$", line):
                self.out.append("<hr>")
                self.i += 1
            elif re.match(r"^\s*[-*]\s+", line):
                self.listing(ordered=False)
            elif re.match(r"^\s*\d+\.\s+", line):
                self.listing(ordered=True)
            else:
                self.paragraph()
            if self.i == before:
                # No handler consumed the line. Emit it escaped and move on:
                # an unsupported construct should show up as visible text, never
                # spin the loop forever.
                self.out.append(f"<p>{inline(self.lines[before].strip())}</p>")
                self.i += 1
        if self.open_section:
            self.out.append("</section>")
        return self


def build(md_path, source_label, title=None):
    text = md_path.read_text(encoding="utf-8")
    r = Renderer(text.split("\n")).run()

    # Page name: drop a "Plan:"-style category prefix and any parenthetical
    # subtitle from the H1, keeping the identity half. --title overrides, which
    # is how a published page keeps a stable name when the H1 is reworded.
    name = re.sub(r"\s*\(.*?\)\s*$", "", r.title).strip()
    name = re.sub(r"^(?:plan|design record|rfc)\s*[:—-]\s*", "", name, flags=re.I)
    name = title or name.replace("-", " ") or md_path.stem

    nav = ""
    if r.toc:
        items = "".join(f'<li><a href="#{a}">{html.escape(t)}</a></li>' for t, a in r.toc)
        nav = f'<nav class="index" aria-label="Sections"><h2>Contents</h2><ol>{items}</ol></nav>'

    return f"""<title>{html.escape(name)}</title>
<link rel="stylesheet" href="https://fonts.googleapis.com/css2?family=Archivo:wght@500;600;700\
&family=Source+Serif+4:ital,opsz,wght@0,8..60,400;0,8..60,600;1,8..60,400\
&family=IBM+Plex+Mono:wght@400;500;600&display=swap">
<style>{CSS}</style>
<header class="masthead"><div class="masthead-in">
<p class="eyebrow">Loom &nbsp;·&nbsp; Design Record</p>
<h1>{html.escape(name)}</h1>
<div class="meta">{''.join(r.meta)}</div>
</div></header>
<div class="shell">{nav}<main>{''.join(r.out)}</main></div>
<footer><div class="footer-in">
<p>Generated from <strong>{html.escape(source_label)}</strong> by
<strong>scripts/render-plan-page.py</strong>. The Markdown is the source of record — edit it
there and re-render; edits made to this page are overwritten on the next run.</p>
</div></footer>
"""


def main():
    ap = argparse.ArgumentParser(description=__doc__.split("\n")[0])
    ap.add_argument("markdown", type=pathlib.Path, help="path to a docs/plan-*.md file")
    ap.add_argument("-o", "--out", type=pathlib.Path, required=True, help="HTML file to write")
    ap.add_argument("-t", "--title", help="override the page name (keeps a published page's "
                                          "title stable when the H1 is reworded)")
    args = ap.parse_args()

    if not args.markdown.is_file():
        sys.exit(f"not a file: {args.markdown}")

    try:
        label = str(args.markdown.resolve().relative_to(pathlib.Path.cwd()))
    except ValueError:
        label = args.markdown.name

    out = build(args.markdown, label, args.title)
    args.out.write_text(out, encoding="utf-8")
    print(f"{args.markdown} -> {args.out}  ({len(out):,} bytes)")


if __name__ == "__main__":
    main()
