import csv
import re
import pprint
import json
from codeowners import CodeOwners

# The fact that this is getting big and untested is bad discipline
# TODO: Create an integration test with various - possibly take a real sarif, pare it down, and use that as a test

findings = {}
jfile = open('go.sarif')
jsonread = json.loads(jfile.read())
jfile.close()

# Read codeowners to figure out where to assign responsibility
f = open('.github/CODEOWNERS', 'r')
owners = CodeOwners(f.read())
f.close()

# Find where the cryptographic footprint short descriptions are
if 'rules' in jsonread["runs"][0]['tool']['driver'] and len(jsonread["runs"][0]['tool']['driver']['rules']) != 0:
  cfrules = []
  for i in jsonread["runs"][0]['tool']['driver']['rules']:
    if 'security' in i['properties']['tags']:
      cfrules += [i]
else:
  # TODO: Refactor qlpack to have a unique name than jank regex search in location
  for extension in jsonread["runs"][0]['tool']['extensions']:
    if re.match(r".*klai/cryptographic-footprint.*", extension['locations'][0]['uri']) is not None:
      cfrules = extension['rules']
      break

# Create a ruleId / short description lookup
rulelookup = {}
for row in cfrules:
  rulelookup[row['id']] = row['shortDescription']['text']

for row in jsonread["runs"][0]["results"]:
  # Skip anything not related to cryptographic footprint
  if not row['ruleId'] in rulelookup:
    continue

  # Get some baseline items - owner, message, location, line, description
  message = row["message"]["text"]
  location = row["locations"][0]["physicalLocation"]["artifactLocation"]['uri']
  line = row["locations"][0]["physicalLocation"]["region"]['startLine']
  description = rulelookup[row['ruleId']]
  if len(owners.of(location)) == 0:
    owner = "No Owner"
  else:
    owner = " ".join(owners.of(location)[0])

  # Create a dict for the owner, if one doesn't exist yet
  if owner not in findings:
    findings[owner] = {}

  # Matching on a cryptographic rule
  # TODO: This is a bit brittle - replace with a check on `ruleId` for `/cf-`
  # Can't get rid of this immediately since we use this to establish where to bucket this in packages
  packagematch = re.match(r"Detected (.*) from (.*)", message)
  if packagematch is not None:
    approvetype = description.split(" ")[0]
    if approvetype not in findings[owner]:
      findings[owner][approvetype] = {}
    findingdict = findings[owner][approvetype]
    packagename = packagematch.group(2)
    # Grab package name and throw the finding into an array with that name in the findings[owner]
    if packagename not in findingdict:
      findingdict[packagename] = []
    findingdict[packagename] += [[message, '{}:{}'.format(location, line)]]

  # Matching on Go Mod
  GMmatch = re.match(r"Go Mod Library Check", description)
  if GMmatch is not None:
    if 'PossibleLibrary' not in findings[owner]:
      findings[owner]['PossibleLibrary'] = []
    findings[owner]['PossibleLibrary'] += [[message, '{}:{}'.format(location, line)]]

  # Matching on Go Import
  GImatch = re.match(r"Go Import Library Check", description)
  if GImatch is not None:
    if 'PossibleImport' not in findings[owner]:
      findings[owner]['PossibleImport'] = []
    findings[owner]['PossibleImport'] += [[message, '{}:{}'.format(location, line)]]

ApprovedCount = 0
for i in findings:
  if 'Approved' in findings[i]:
    for j in findings[i]['Approved']:
      ApprovedCount += len(findings[i]['Approved'][j])

DisallowedCount = 0
for i in findings:
  if 'Disallowed' in findings[i]:
    for j in findings[i]['Disallowed']:
      DisallowedCount += len(findings[i]['Disallowed'][j])

MiscellaneousCount = 0
for i in findings:
  if 'Miscellaneous' in findings[i]:
    for j in findings[i]['Miscellaneous']:
      MiscellaneousCount += len(findings[i]['Miscellaneous'][j])

csvtowrite = open('summary.txt', 'w')
csvwrite = csv.writer(csvtowrite, delimiter=',', quotechar='"')

LibraryCount = 0
for i in findings:
  if 'PossibleLibrary' in findings[i]:
    LibraryCount += len(findings[i]['PossibleLibrary'])

# libraryCount = len(findings['PossibleLibrary'])
LibraryUsageStr = "Found {} crypto libraries:".format(LibraryCount)
csvwrite.writerow(["#" * len(LibraryUsageStr)])
csvwrite.writerow([LibraryUsageStr])
csvwrite.writerow(["#" * len(LibraryUsageStr)])
for i in findings:
  if 'PossibleLibrary' in findings[i]:
    csvwrite.writerow([])
    csvwrite.writerow(["=" * (len(i) + 1)])
    csvwrite.writerow(["{}".format(i)])
    csvwrite.writerow(["=" * (len(i) + 1)])
    for j in findings[i]['PossibleLibrary']:
      csvwrite.writerow(j)

csvwrite.writerow([])

ImportCount = 0
for i in findings:
  if 'PossibleImport' in findings[i]:
    ImportCount += len(findings[i]['PossibleImport'])

ImportUsageStr = "Found {} possible imports:".format(ImportCount)
csvwrite.writerow(["#" * len(ImportUsageStr)])
csvwrite.writerow([ImportUsageStr])
csvwrite.writerow(["#" * len(ImportUsageStr)])
for i in findings:
  if 'PossibleImport' in findings[i]:
    csvwrite.writerow([])
    csvwrite.writerow(["=" * (len(i) + 1)])
    csvwrite.writerow(["{}".format(i)])
    csvwrite.writerow(["=" * (len(i) + 1)])
    for j in findings[i]['PossibleImport']:
      m = re.match("Possible crypto import: (.*)", j[0])
      csvwrite.writerow(["{}".format(m.group(1)), j[1]])


csvwrite.writerow([])
OpUsageStr = "Found {} strong cryptographic operation usages".format(ApprovedCount)
csvwrite.writerow(["#" * len(OpUsageStr)])
csvwrite.writerow([OpUsageStr])
csvwrite.writerow(["#" * len(OpUsageStr)])

for a in findings:
  if 'Approved' in findings[a]:
    csvwrite.writerow([])
    csvwrite.writerow(["=" * (len(a) + 1)])
    csvwrite.writerow(["{}".format(a)])
    csvwrite.writerow(["=" * (len(a) + 1)])
    for i in findings[a]['Approved']:
      csvwrite.writerow([])
      csvwrite.writerow(["{}".format(i)])
      csvwrite.writerow(["-" * len(i)])
      for j in findings[a]['Approved'][i]:
        m = re.match("Detected (.*) from (.*)", j[0])
        csvwrite.writerow(["{}".format(m.group(1)), j[1]])

csvwrite.writerow([])

WeakUsageStr = "Found {} weak or disallowed cryptographic operation usages".format(DisallowedCount)
csvwrite.writerow(["#" * len(WeakUsageStr)])
csvwrite.writerow([WeakUsageStr])
csvwrite.writerow(["#" * len(WeakUsageStr)])

for a in findings:
  if 'Disallowed' in findings[a]:
    csvwrite.writerow([])
    csvwrite.writerow(["=" * (len(a) + 1)])
    csvwrite.writerow(["{}".format(a)])
    csvwrite.writerow(["=" * (len(a) + 1)])
    for i in findings[a]['Disallowed']:
      csvwrite.writerow([])
      csvwrite.writerow(["{}".format(i)])
      csvwrite.writerow(["-" * len(i)])
      for j in findings[a]['Disallowed'][i]:
        m = re.match("Detected (.*) from (.*)", j[0])
        csvwrite.writerow(["{}".format(m.group(1)), j[1]])

csvwrite.writerow([])

MiscUsageStr = "Found {} miscellaneous usages".format(MiscellaneousCount)
csvwrite.writerow(["#" * len(MiscUsageStr)])
csvwrite.writerow([MiscUsageStr])
csvwrite.writerow(["#" * len(MiscUsageStr)])

for a in findings:
  if 'Miscellaneous' in findings[a]:
    csvwrite.writerow([])
    csvwrite.writerow(["=" * (len(a) + 1)])
    csvwrite.writerow(["{}".format(a)])
    csvwrite.writerow(["=" * (len(a) + 1)])
    for i in findings[a]['Miscellaneous']:
      csvwrite.writerow([])
      csvwrite.writerow(["{}".format(i)])
      csvwrite.writerow(["-" * len(i)])
      if re.match('.*rand.*', i) is not None:
        csvwrite.writerow(["Please ensure that `rand` invocations that are called from packages for their purpose"])
        csvwrite.writerow(["For example - when cryptographically secure randomness is needed we call `crypto/rand`"])
        csvwrite.writerow(["In the case of FIPS-140 compliance ensure that `rand` is from a FIPS compliant package"])
      csvwrite.writerow([])
      for j in findings[a]['Miscellaneous'][i]:
        m = re.match("Detected (.*) from (.*)", j[0])
        csvwrite.writerow(["{}".format(m.group(1)), j[1]])

csvwrite.writerow([])

csvtowrite.close()
