#!/usr/bin/env python3
"""
Build script to generate a single standalone HTML file from markdown + CSS.
Uses pandoc to convert markdown to HTML and embeds everything into one file.
"""

import os
import subprocess
import sys
from pathlib import Path


def run_command(cmd):
    """Run a shell command and return output."""
    result = subprocess.run(cmd, shell=True, capture_output=True, text=True)
    if result.returncode != 0:
        print(f"Error running command: {cmd}")
        print(f"stderr: {result.stderr}")
        sys.exit(1)
    return result.stdout


def check_pandoc():
    """Check if pandoc is installed."""
    try:
        subprocess.run(['pandoc', '--version'], capture_output=True, check=True)
        return True
    except (subprocess.CalledProcessError, FileNotFoundError):
        return False


def read_file(filepath):
    """Read file contents."""
    with open(filepath) as f:
        return f.read()


def main():
    script_dir = Path(__file__).parent
    content_md = script_dir / 'content.md'
    style_css = script_dir / 'style.css'
    output_file = script_dir / 'index.html'

    # Check files exist
    if not content_md.exists():
        print(f"Error: {content_md} not found")
        sys.exit(1)
    if not style_css.exists():
        print(f"Error: {style_css} not found")
        sys.exit(1)

    # Check pandoc is installed
    if not check_pandoc():
        print("Error: pandoc is not installed")
        print("Install with: brew install pandoc")
        sys.exit(1)

    print("Building standalone HTML...")

    # Lint markdown
    print("  Linting markdown with markdownlint...")
    result = subprocess.run(['npx', 'markdownlint-cli2', str(content_md)], capture_output=True, text=True)
    if result.returncode != 0:
        print("Linting errors found:")
        print(result.stdout if result.stdout else result.stderr)
        sys.exit(1)

    # Convert markdown to HTML using pandoc
    print("  Converting markdown to HTML with pandoc...")
    md_content = read_file(content_md)

    # Use pandoc to convert markdown to HTML fragment
    # --from markdown: input format
    # --to html5: output format
    # --standalone: include document structure
    # --metadata pagetitle: set page title
    # --css: inline CSS file
    html_output = run_command(
        f'pandoc "{content_md}" --from markdown --to html5 '
        f'--standalone --metadata pagetitle="Datadog Agent O11y Signals & AI Gadgets" '
        f'--css "{style_css}"'
    )

    # Read CSS content to embed it
    css_content = read_file(style_css)

    # Replace the <link> tag with embedded <style> tag
    # Use regex to handle variations in the link tag format
    import re

    html_output = re.sub(
        r'<link rel="stylesheet" href="[^"]*style\.css" />', f'<style>\n{css_content}\n</style>', html_output
    )

    # Add back-to-top button and JavaScript
    js_additions = '''
<script>
// Back to top button functionality
const backToTopButton = document.createElement('a');
backToTopButton.href = '#';
backToTopButton.className = 'back-to-top';
backToTopButton.id = 'backToTop';
backToTopButton.textContent = '↑';
document.body.appendChild(backToTopButton);

// Add back-to-top button styling
const style = document.createElement('style');
style.textContent = `
    .back-to-top {
        position: fixed;
        bottom: 2rem;
        right: 2rem;
        display: none;
        width: 3rem;
        height: 3rem;
        background: var(--primary, #632ca6);
        color: white;
        border-radius: 50%;
        text-align: center;
        line-height: 3rem;
        text-decoration: none;
        font-size: 1.5rem;
        z-index: 1000;
        transition: background 0.3s, opacity 0.3s;
    }

    .back-to-top:hover {
        background: var(--secondary, #774aa4);
    }

    .back-to-top.visible {
        display: block;
    }
`;
document.head.appendChild(style);

// Show/hide button on scroll
window.addEventListener('scroll', () => {
    if (window.scrollY > 500) {
        backToTopButton.classList.add('visible');
    } else {
        backToTopButton.classList.remove('visible');
    }
});

// Scroll to top on click
backToTopButton.addEventListener('click', (e) => {
    e.preventDefault();
    window.scrollTo({ top: 0, behavior: 'smooth' });
});

</script>
'''

    # Wrap body content in document container div for layout styling
    html_output = html_output.replace('<body>', '<body><div class="document">')

    # Insert JavaScript before closing body tag (after closing document div)
    html_output = html_output.replace('</body>', '</div>' + js_additions + '</body>')

    # Write output file
    print(f"  Writing to {output_file}...")
    with open(output_file, 'w') as f:
        f.write(html_output)

    # Get file size
    file_size = os.path.getsize(output_file)
    print(f"✓ Build complete: {output_file} ({file_size / 1024:.1f} KB)")
    print("\nYour standalone HTML file is ready for distribution!")


if __name__ == '__main__':
    main()
