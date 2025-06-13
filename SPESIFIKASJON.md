## Spesifikasjon for Mat- og Symptomdagbok Programvare

**Formål:**
Programvaren skal la brukeren registrere måltider og symptomer for å kunne spore mulige sammenhenger mellom mat og helseplager.

**Kjernfunksjoner:**
1. **Registrering av måltider:**
   - Brukeren kan legge inn matvarer, drikke og kosttilskudd.
   - Mulighet til å velge fra forhåndsdefinerte lister eller skrive inn egne verdier.

2. **Registrering av symptomer:**
   - Brukeren kan loggføre symptomer som oppstår.
   - Mulighet til å velge fra forhåndsdefinerte lister eller skrive inn egne symptomer.

3. **Tidsstempling:**
   - Hver registrering (måltid eller symptom) skal ha et tidsstempel.
   - Automatisk tidspunktregistrering, med mulighet for manuell justering.

4. **Notatfelt:**
   - Et fritekstfelt for ekstra detaljer eller observasjoner tilknyttet hver registrering.

5. **Rapportering og analyse:**
   - Generer oversikter over registrerte data for valgte tidsperioder.
   - Visualisering av data for å avdekke mønstre eller sammenhenger.

6. **Datahåndtering:**
   - Mulighet til å lagre, redigere og slette registreringer.
   - Funksjon for å eksportere data til en fil.

7. **Personvern:**
   - Sørge for at brukerens data er private og sikre.
   - Data lagres lokalt eller på en sikker server.

**Brukervennlighet:**
- Enkelt og intuitivt grensesnitt.
- Tydelige instruksjoner for å registrere data.
- Få trinn for å legge inn informasjon.

**Teknologivalg:**
- Programmeringsspråk: Go
- Database: SQLite
