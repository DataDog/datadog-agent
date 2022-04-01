import csv
import re
import pprint
import json

findings = {'Approved': {}, 'Disallowed': {}, 'Miscellaneous': {}, 'PossibleLibrary': [], 'PossibleImport': []}
jfile = open('go.sarif')
jsonread = json.loads(jfile.read())
jfile.close()

# Find where the cryptographic footprint short descriptions are
# TODO: Refactor qlpack to have a unique name than jank regex search in location
for extension in jsonread["runs"][0]['tool']['extensions']:
  if re.match(r".*klai/cryptographic-footprint.*", extension['locations'][0]['uri']) is not None:
    cfextension = extension
    break

# Create a ruleId / short description lookup
rulelookup = {}
for row in cfextension['rules']:
  rulelookup[row['id']] = row['shortDescription']['text']

for row in jsonread["runs"][0]["results"]:
  if not row['ruleId'] in rulelookup:
    continue
  message = row["message"]["text"]
  location = row["locations"][0]["physicalLocation"]["artifactLocation"]['uri']
  startline = row["locations"][0]["physicalLocation"]["region"]['startLine']
  description = rulelookup[row['ruleId']]
  packagematch = re.match(r"Detected (.*) from (.*)", message)
  if packagematch is not None:
    approvetype = description.split(" ")[0]
    findingdict = findings[approvetype]
    packagename = packagematch.group(2)
    if packagename not in findingdict:
      findingdict[packagename] = []
    findingdict[packagename] += [[message, '{}:{}'.format(location, startline)]]
  GMmatch = re.match(r"Go Mod Library Check", description)
  if GMmatch is not None:
    findings['PossibleLibrary'] += [[message, '{}:{}'.format(location, startline)]]
  GImatch = re.match(r"Go Import Library Check", description)
  if GImatch is not None:
    findings['PossibleImport'] += [[message, '{}:{}'.format(location, startline)]]

ApprovedCount = 0
for i in findings['Approved']:
  for j in findings['Approved'][i]:
      ApprovedCount += 1

DisallowedCount = 0
for i in findings['Disallowed']:
  for j in findings['Disallowed'][i]:
      DisallowedCount += 1

MiscellaneousCount = 0
for i in findings['Miscellaneous']:
  for j in findings['Miscellaneous'][i]:
      MiscellaneousCount += 1

csvtowrite = open('summary.txt', 'w')
csvwrite = csv.writer(csvtowrite, delimiter=',', quotechar='"')
csvwrite.writerow(["Found {} crypto libraries:".format(len(findings['PossibleLibrary']))])
for i in findings['PossibleLibrary']:
    csvwrite.writerow(i)

csvwrite.writerow([])

csvwrite.writerow(["Found {} possible imports:".format(len(findings['PossibleImport']))])
for i in findings['PossibleImport']:
    m = re.match("Possible crypto import: (.*)", i[0])
    csvwrite.writerow(["{}".format(m.group(1)), i[1]])

csvwrite.writerow(["======================================================================================="])
csvwrite.writerow(["Found {} strong cryptographic operation usages across {} types:".format(ApprovedCount, len(findings['Approved']))])
csvwrite.writerow(["======================================================================================="])
for i in findings['Approved']:
  csvwrite.writerow([])
  csvwrite.writerow(["--------------------------{}---------------------------".format(i)])
  csvwrite.writerow([])
  for j in findings['Approved'][i]:
    m = re.match("Detected (.*) from (.*)", j[0])
    csvwrite.writerow(["{}".format(m.group(1)), j[1]])

csvwrite.writerow([])

csvwrite.writerow(["======================================================================================="])
csvwrite.writerow(["Found {} weak or disallowed cryptographic operation usages across {} types:".format(DisallowedCount, len(findings['Disallowed']))])
csvwrite.writerow(["======================================================================================="])
for i in findings['Disallowed']:
  csvwrite.writerow([])
  csvwrite.writerow(["--------------------------{}---------------------------".format(i)])
  csvwrite.writerow([])
  for j in findings['Disallowed'][i]:
    m = re.match("Detected (.*) from (.*)", j[0])
    csvwrite.writerow(["{}".format(m.group(1)), j[1]])

csvwrite.writerow([])

csvwrite.writerow(["======================================================================================="])
csvwrite.writerow(["Found {} miscellaneous usages across {} modules:".format(MiscellaneousCount, len(findings['Miscellaneous']))])
csvwrite.writerow(["======================================================================================="])
for i in findings['Miscellaneous']:
  csvwrite.writerow([])
  csvwrite.writerow(["--------------------------{}---------------------------".format(i)])
  if re.match('.*rand.*', i) is not None:
    csvwrite.writerow(["Please ensure that `rand` invocations that are called from packages for their purpose"])
    csvwrite.writerow(["For example - when cryptographically secure randomness is needed we call `crypto/rand`"])
    csvwrite.writerow(["In the case of FIPS-140 compliance ensure that `rand` is from a FIPS compliant package"])
  csvwrite.writerow([])
  for j in findings['Miscellaneous'][i]:
    m = re.match("Detected (.*) from (.*)", j[0])
    csvwrite.writerow(["{}".format(m.group(1)), j[1]])

csvwrite.writerow([])

csvtowrite.close()
