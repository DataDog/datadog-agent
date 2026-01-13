import csv
import sys

def parse_size(size_str):
    """Convert size string to MB"""
    size_str = size_str.strip()
    if 'GB' in size_str:
        return float(size_str.replace('GB', '')) * 1024
    elif 'MB' in size_str:
        return float(size_str.replace('MB', ''))
    else:
        return 0

# Read competitor data
competitors = {}
try:
    with open('competitors.csv', 'r') as f:
        reader = csv.DictReader(f)
        for row in reader:
            size = parse_size(row['Compressed Size'])
            if size > 0:
                competitors[row['Product']] = size
except:
    pass

# Read Datadog versions
dd_versions = {}
try:
    with open('datadog_versions.csv', 'r') as f:
        reader = csv.DictReader(f)
        for row in reader:
            size = parse_size(row['Compressed Size'])
            if size > 0:
                dd_versions[row['Version']] = size
except:
    pass

# Print comparison table
if competitors:
    dd_size = competitors.get('Datadog Agent', 1000)
    
    print("\n" + "="*60)
    print("SIZE COMPARISON MATRIX")
    print("="*60)
    print(f"{'Product':<25} {'Size (MB)':<12} {'vs Datadog':<12} {'Category'}")
    print("-"*60)
    
    sorted_competitors = sorted(competitors.items(), key=lambda x: x[1])
    
    for product, size in sorted_competitors:
        ratio = size / dd_size if dd_size > 0 else 0
        
        # Categorize
        if 'Prometheus' in product or 'Exporter' in product:
            category = 'Metrics'
        elif 'beat' in product.lower():
            category = 'Elastic'
        elif 'OTel' in product:
            category = 'OpenTelemetry'
        elif 'Fluent' in product or 'Vector' in product:
            category = 'Logs'
        else:
            category = 'Full Platform'
        
        comparison = f"{ratio:.2f}x" if product != 'Datadog Agent' else "baseline"
        print(f"{product:<25} {size:<12.1f} {comparison:<12} {category}")
    
    # Summary statistics
    other_sizes = [s for p, s in competitors.items() if p != 'Datadog Agent']
    if other_sizes:
        avg_size = sum(other_sizes) / len(other_sizes)
        min_size = min(other_sizes)
        max_size = max(other_sizes)
        
        print("\n" + "="*60)
        print("STATISTICS")
        print("="*60)
        print(f"Datadog Agent Size: {dd_size:.1f} MB")
        print(f"Competitor Average: {avg_size:.1f} MB")
        print(f"Smallest Competitor: {min_size:.1f} MB")
        print(f"Largest Competitor: {max_size:.1f} MB")
        print(f"Datadog vs Average: {(dd_size/avg_size):.1f}x larger")

# Print version trend
if dd_versions:
    print("\n" + "="*60)
    print("DATADOG VERSION TREND")
    print("="*60)
    
    versions = sorted(dd_versions.items())
    if len(versions) > 1:
        first_size = versions[0][1]
        last_size = versions[-1][1]
        growth = ((last_size - first_size) / first_size) * 100
        
        for version, size in versions:
            growth_from_first = ((size - first_size) / first_size) * 100 if first_size > 0 else 0
            print(f"v{version:<10} {size:<10.1f} MB   (+{growth_from_first:.1f}% from v{versions[0][0]})")
        
        print(f"\nTotal growth: {growth:.1f}%")
        print(f"Average version increase: {growth/(len(versions)-1):.1f}%")
