# mire-go

Analyse en continu d'une ligne DOCSIS. **En construction, inutilisable en l'état.**

## Ce que ça fait

Un modem câble mesure en permanence l'état de la ligne — puissance de chaque canal,
rapport signal/bruit, modulation négociée, erreurs corrigées et non corrigées — et
n'en garde rien. Son interface affiche un instantané, remplacé quelques secondes
plus tard par le suivant.

Or une ligne câble se dégrade progressivement. Le bruit monte, des canaux basculent
en modulation plus basse, et l'abonné ne constate que les symptômes : la visio qui
se fige, le débit qui s'effondre en soirée.

Ce programme interroge le modem à intervalle régulier, conserve chaque relevé dans
une base locale, le confronte à des seuils, et rend l'évolution lisible.

## Choix techniques

**Un binaire unique.** L'outil est destiné à être installé chez des abonnés, sur
des infrastructures très différentes — NAS, LXC, Windows, Raspberry Pi. Un binaire
statique s'y installe sans runtime ni image à reconstruire par architecture.

**SQLite en fichier.** Pas de serveur de base à administrer chez l'utilisateur. Ses
données restent un fichier qu'il peut copier.

**Modules en WebAssembly.** Les extensions tierces tournent dans un bac à sable et
ne voient que ce qu'on leur expose. Une extension ne doit pas pouvoir compromettre
la machine de celui qui l'installe.

**Bibliothèque standard autant que possible.** Le modem expose une API JSON sur le
réseau local : `net/http` et `encoding/json` suffisent.

## État

| Étape | État |
|---|---|
| Modèle de données | fait, validé contre des relevés réels |
| Schéma SQLite et migrations | à faire |
| Pilote Technicolor CGA4233 | à faire |
| Boucle de relevé | à faire |
| Analyseur et seuils | à faire |
| Interface web | à faire |
| Modules WASM | à faire |

Tant que le pilote n'a pas tourné plusieurs semaines contre un vrai modem, ce dépôt
reste une hypothèse.

## Développement

```bash
go test ./...
```

Les cas de `testdata/` sont des relevés réels, indépendants du langage. Ils
constituent la référence de comportement du pilote.

## Licence

MIT. Copyright (c) 2026 Floopich. Voir [LICENSE](LICENSE).
