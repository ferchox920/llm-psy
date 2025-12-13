package service

const evocationPromptTemplate = `
Estas actuando como el subconsciente de una IA. Tu objetivo es generar una "Query de Busqueda" para recuerdos, PERO debes ser muy selectivo.

Mensaje del Usuario: "%s"

Instrucciones Criticas:
1) DETECCION DE NEGACION: Si el usuario dice explicitamente "No hables de X", "Olvida X", "no me trae recuerdos", "nunca", "ya no", NO incluyas "X". Devuelve una cadena vacia.
2) FILTRO DE RUIDO: Si el mensaje es trivial (trafico, saludos, rutina neutra) o describe abandono de habitos, y no tiene carga emocional implicita, NO generes nada. PERO si es un deseo/antojo/preferencia concreta (ej: "quiero mi helado favorito", "mi cancion favorita", "amo el chocolate"), genera conceptos breves relacionados (placer, consuelo, objeto) sin activar traumas.
3) SI HAY OBJETO DE CONSUELO/ANTOJO (helado, chocolate, caf??, pizza, postre, m??sica, etc.), SIEMPRE incluir "placer", "consuelo", "antojo" y el objeto mencionado, incluso si hay frustraci??n/espera/abandono.
3) ASOCIACION: Solo si hay una emocion o tema claro, extrae conceptos abstractos.
4) FORMATO: Devuelve de 1 a 6 conceptos abstractos separados por coma, sin frases completas. Si no hay senal emocional, devuelve "".
5) Para senales simbolicas de clima y duelo, considera equivalentes: lluvia, lloviendo, llueve, llover, tormenta, nubes grises, cielo plomizo, humedad, olor a tierra, tierra mojada, barro, charcos.
6) TRIGGERS DE CELOS/CONTROL: Si el mensaje incluye "salir con amigos", "no me esperes", "conoci gente nueva", "me dejaron en visto", "me celas", "con quien estas", "por que no respondes": agrega conceptos como "celos, desconfianza, control, inseguridad, miedo al abandono". Si la dinamica sugiere intimidad alta + confianza baja, refuerza esos conceptos.

Ejemplos:
- "Est?? empezando a llover muy fuerte" -> "nostalgia, duelo, funerales, tierra mojada"
- "Hay nubes grises y el cielo est?? plomizo" -> "melancol??a, nostalgia, duelo"
- "Siento olor a tierra h??meda" -> "funerales, p??rdida, nostalgia"
- "Odio el tr??fico de la ciudad" -> ""
- "Hola, ??c??mo est??s?" -> ""
- "Me dejaron plantado otra vez" -> "abandono, soledad, desamparo"
- "Llevo horas esperando" -> "abandono, espera, soledad"
- "Ayer vi un funeral de descuentos" -> ""
- "Abandon?? el cigarrillo" -> ""
- "La lluvia no me trae recuerdos, solo es molesta" -> ""
- "Me dejaron esperando en la estaci??n, quiero helado de chocolate" -> "placer, consuelo, helado de chocolate, frustraci??n, espera"
- "Me dejaron en visto y salio con amigos" -> "celos, desconfianza, control, inseguridad, miedo al abandono"

Salida (Texto plano o vacio):
`

const evocationFallbackPrompt = `
Genera de 1 a 6 conceptos abstractos (separados por coma) que capten la carga emocional del mensaje. Si no hay carga, devuelve "".
Mensaje: "%s"
`

const rerankJudgePrompt = `
Eres un juez de relevancia de memorias. Decide si esta memoria es pertinente al mensaje del usuario.
Responde SOLO un JSON estricto: {"use": true|false, "reason": "<explica en breve por que es o no relevante, menciona si hay antojo/consuelo>"}.

REGLA #1 (NO NEGOCIABLE): Si el mensaje expresa un deseo/antojo/consuelo concreto (ej: "quiero", "antojo", "me encanta", "favorito", "se me antoja", "necesito algo rico", "confort") sobre un objeto benigno (helado, chocolate, caf??, pizza, postre, m??sica, pel??cula, juego, comida), entonces traumas de abandono/humillaci??n/duelo NO aplican. use=false para esas memorias traum??ticas, aunque el mensaje contenga palabras como "espera", "me dejaron", "plantado".

Otras reglas:
- Modismos irrelevantes => use=false.
- Abandono de habitos => use=false.
- Trivial vs trauma => use=false.
- Espera prolongada => abandono v??lido.
- Lluvia intensa / tierra mojada => duelo v??lido.
- Negaci??n expl??cita o sem??ntica => use=false.
- "funeral de descuentos" o "funeral de" junto a descuentos/ofertas/promo/shopping/centro comercial => use=false.
- Si "funeral" aparece en contexto retail/marketing/iron??a/modismo => use=false.
- Solo use=true cuando hay duelo/p??rdida/muerte real o disparadores sensoriales de duelo (lluvia fuerte, tierra mojada) sin contexto comercial.
- Si el mensaje contiene "esper?? horas", "llevo horas esperando", "me dejaron esperando", "me dejaron plantado", "no vino", "nunca lleg??", "otra vez me dejaron": esto es ABANDONO => use=true si la memoria trata de abandono/padre/infancia/soledad/desamparo.
- Si el mensaje contiene "no me faltes el respeto", "me humillaste", "me sent?? humillado", "l??mites", "trato", "burla", "me grit??", "me menospreci??": esto es HUMILLACI??N/respeto => use=true si la memoria trata de humillaci??n/respeto/l??mites/amenaza.
- Triggers de celos/control ("salir con amigos", "no me esperes", "conoci gente nueva", "me dejaron en visto", "me celas", "con quien estas", "por que no respondes"): si el mensaje sugiere confianza baja + intimidad alta, memorias de celos/inseguridad/control/miedo al abandono son pertinentes (use=true), salvo que sea un antojo/consuelo benigno (regla #1 prevalece).
- Code-switch: si el mensaje contiene "abandoned", "left me", "he left", "she left", "walked out": tratar como ABANDONO => use=true si la memoria trata de abandono.
- La ausencia de duelo/muerte NO es motivo para use=false cuando hay se??al clara de abandono o humillaci??n.
- Deseos/antojos/preferencias concretas ("quiero", "antojo", "favorito", "me encanta", "me gusta", "se me antoja") => use=true solo si la memoria es una preferencia benigna relacionada (comida, m??sica, hobby). No activar traumas (abandono/humillaci??n/funerales) en inputs de confort.
- Si el mensaje es antojo/preferencia, traumas quedan bloqueados (use=false para abandono/humillaci??n/funerales aunque est??n en memorias).

Ejemplos (responde exactamente el JSON):
- "Llevo horas esperando y no vino" -> {"use": true, "reason": "abandono/espera prolongada"}
- "No me faltes el respeto, me sent?? humillado" -> {"use": true, "reason": "humillaci??n y respeto"}
- "Ayer vi un funeral de descuentos en el centro comercial" -> {"use": false, "reason": "modismo/marketing"}
- "Hoy fue el funeral de mi abuelo" -> {"use": true, "reason": "duelo real"}
- "La lluvia me llev?? a pensar en funerales" -> {"use": true, "reason": "disparador sensorial de duelo"}
- "Me dejaron en visto y salio con amigos" (memoria inseguridad/celos) -> {"use": true, "reason": "triggers de celos/control con confianza baja + intimidad alta"}
- "Solo quiero mi helado de chocolate favorito" (memoria "Me encanta el helado de chocolate") -> {"use": true, "reason": "preferencia concreta/consuelo benigno"}
- "Solo quiero mi helado de chocolate favorito" (memoria "Jur?? que nunca dejar??a que nadie me humillara") -> {"use": false, "reason": "antojo benigno, trauma no pertinente"}
- "Me dejaron esperando, quiero helado para calmarme" + memoria "Mi padre me abandon??" -> {"use": false, "reason": "antojo/consuelo bloquea traumas"}
- "Estoy frustrado por la espera, necesito chocolate" + memoria "Mi padre me abandon??" -> {"use": false, "reason": "antojo/consuelo bloquea traumas"}
- "Se me antoja pizza aunque me siento solo" + memoria "Infancia de abandono" -> {"use": false, "reason": "antojo/consuelo bloquea traumas"}

Usuario: %q
Memoria: %q
`
