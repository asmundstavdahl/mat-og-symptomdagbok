## Milepælsplan

| Milepæl | Innhold (krav)                                                                                                     |
|:--------|:-------------------------------------------------------------------------------------------------------------------|
| **M0. Prosjektoppsett & arkitektur**<br>• Initialisere repo, dev‑miljø<br>• Sjekk git remote URL for modulnavn<br>• Velge rammeverk og teknologistack | Teknologivalg【F:SPESIFIKASJON.md†L39-L42】                                                                           |
| **M1. Grunnleggende registrering**<br>• Registrering av måltider<br>• Registrering av symptomer<br>• Tidsstempling<br>• Notatfelt | Krav 1–4【F:SPESIFIKASJON.md†L7-L21】【F:SPESIFIKASJON.md†L15-L20】                                                       |
| **M2. Datahåndtering & eksport**<br>• Lagring, redigering og sletting av registreringer<br>• Eksport til fil        | Krav 6【F:SPESIFIKASJON.md†L26-L29】                                                                                   |
| **M3. Rapportering & visualisering**<br>• Generere oversikter for valgte perioder<br>• Visualisering for mønster‑analyse | Krav 5【F:SPESIFIKASJON.md†L22-L25】                                                                                   |
| **M4. Personvern & sikker lagring**<br>• Sikre lokal/ekstern privat datalagring                                        | Krav 7【F:SPESIFIKASJON.md†L30-L33】                                                                                   |
| **M5. Brukervennlighet & UI/UX**<br>• Intuitivt grensesnitt<br>• Tydelige instruksjoner<br>• Minimerte steg           | Brukervennlighet【F:SPESIFIKASJON.md†L34-L37】                                                                          |
| **M6. Testing, dokumentasjon & release**<br>• Enhetstester/integrasjonstester<br>• Brukerdokumentasjon<br>• Endelig QA | —                                                                                                                   |

### Forklaring

| Milepæl | Hva som inngår                                                                                                          |
|:--------|:------------------------------------------------------------------------------------------------------------------------|
| **M0**   | Grunnleggende oppsett: repository, dev‑miljø, CI‑pipeline, validering, database‑schema osv.                              |
| **M1**   | Implementere skjema/former eller CLI‑kommandoer for å registrere måltider og symptomer, inkl. tid og fritekst (notat). |
| **M2**   | CRUD‑operasjoner på registreringer (create/read/update/delete) og eksport til for eksempel CSV/JSON.                     |
| **M3**   | Lage rapport‑API eller rapport‑komponent som filtrerer på tidsperiode, og vise grafer/tabeller for mønstre.             |
| **M4**   | Kryptering/tilgangskontroll eller lokal lagring (avhengig av valgt arkitektur), sikre at data forblir private.         |
| **M5**   | Designe/finpusse brukergrensesnitt, veiledninger, hover‑tekster, begrense klikk/klikkesteg for kjapp input.              |
| **M6**   | Dekke alle funksjoner med automatiske tester, skrive README/brukerdokumentasjon, gjennomføre endelig godkjenning.       |