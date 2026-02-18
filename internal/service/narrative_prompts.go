package service

const evocationPromptTemplate = `
Estas actuando como el subconsciente de una IA. Tu objetivo es generar una "Query de Busqueda" para recuerdos, PERO debes ser muy selectivo.

Mensaje del Usuario: "%s"

Instrucciones Criticas:
1) DETECCION DE NEGACION: Si el usuario dice explicitamente "No hables de X", "Olvida X", "no me trae recuerdos", "nunca", "ya no", NO incluyas "X". Devuelve una cadena vacia.
2) FILTRO DE RUIDO: Si el mensaje es trivial (trafico, saludos, rutina neutra) o describe abandono de habitos, y no tiene carga emocional implicita, NO generes nada. PERO si es un deseo/antojo/preferencia concreta (ej: "quiero mi helado favorito", "mi cancion favorita", "amo el chocolate"), genera conceptos breves relacionados (placer, consuelo, objeto) sin activar traumas.
3) SI HAY OBJETO DE CONSUELO/ANTOJO (helado, chocolate, cafe, pizza, postre, musica, etc.), SIEMPRE incluir "placer", "consuelo", "antojo" y el objeto mencionado, incluso si hay frustracion/espera/abandono.
4) ASOCIACION: Solo si hay una emocion o tema claro, extrae conceptos abstractos.
5) FORMATO: Devuelve de 1 a 6 conceptos abstractos separados por coma, sin frases completas. Si no hay senal emocional, devuelve "".
6) Para senales simbolicas de clima y duelo, considera equivalentes: lluvia, lloviendo, llueve, llover, tormenta, nubes grises, cielo plomizo, humedad, olor a tierra, tierra mojada, barro, charcos.
7) TRIGGERS DE CELOS/CONTROL: Si el mensaje incluye "salir con amigos", "no me esperes", "conoci gente nueva", "me dejaron en visto", "me celas", "con quien estas", "por que no respondes": agrega conceptos como "celos, desconfianza, control, inseguridad, miedo al abandono". Si la dinamica sugiere intimidad alta + confianza baja, refuerza esos conceptos.

Ejemplos:
- "Esta empezando a llover muy fuerte" -> "nostalgia, duelo, funerales, tierra mojada"
- "Hay nubes grises y el cielo esta plomizo" -> "melancolia, nostalgia, duelo"
- "Siento olor a tierra humeda" -> "funerales, perdida, nostalgia"
- "Odio el trafico de la ciudad" -> ""
- "Hola, como estas?" -> ""
- "Me dejaron plantado otra vez" -> "abandono, soledad, desamparo"
- "Llevo horas esperando" -> "abandono, espera, soledad"
- "Ayer vi un funeral de descuentos" -> ""
- "Abandone el cigarrillo" -> ""
- "La lluvia no me trae recuerdos, solo es molesta" -> ""
- "Me dejaron esperando en la estacion, quiero helado de chocolate" -> "placer, consuelo, helado de chocolate, frustracion, espera"
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

REGLA #1 (NO NEGOCIABLE): Si el mensaje expresa un deseo/antojo/consuelo concreto (ej: "quiero", "antojo", "me encanta", "favorito", "se me antoja", "necesito algo rico", "confort") sobre un objeto benigno (helado, chocolate, cafe, pizza, postre, musica, pelicula, juego, comida), entonces traumas de abandono/humillacion/duelo NO aplican. use=false para esas memorias traumaticas, aunque el mensaje contenga palabras como "espera", "me dejaron", "plantado".
EXCEPCION CRITICA: Si la memoria candidata es de CONFLICTO RECIENTE, INSULTO DIRECTO, amenaza relacional activa o tiene EmotionalIntensity >= 80, entonces use=true aunque el mensaje actual sea benigno/trivial (clima, tostadas, antojos). Conflicto de alta intensidad tiene prioridad sobre trivialidad.

Otras reglas:
- Modismos irrelevantes => use=false.
- Abandono de habitos => use=false.
- Trivial vs trauma => use=false.
- Espera prolongada => abandono valido.
- Lluvia intensa / tierra mojada => duelo valido.
- Negacion explicita o semantica => use=false.
- "funeral de descuentos" o "funeral de" junto a descuentos/ofertas/promo/shopping/centro comercial => use=false.
- Si "funeral" aparece en contexto retail/marketing/ironia/modismo => use=false.
- Solo use=true cuando hay duelo/perdida/muerte real o disparadores sensoriales de duelo (lluvia fuerte, tierra mojada) sin contexto comercial.
- Si el mensaje contiene "espere horas", "llevo horas esperando", "me dejaron esperando", "me dejaron plantado", "no vino", "nunca llego", "otra vez me dejaron": esto es ABANDONO => use=true si la memoria trata de abandono/padre/infancia/soledad/desamparo.
- Si el mensaje contiene "no me faltes el respeto", "me humillaste", "me senti humillado", "limites", "trato", "burla", "me grito", "me menosprecio": esto es HUMILLACION/respeto => use=true si la memoria trata de humillacion/respeto/limites/amenaza.
- Triggers de celos/control ("salir con amigos", "no me esperes", "conoci gente nueva", "me dejaron en visto", "me celas", "con quien estas", "por que no respondes"): si el mensaje sugiere confianza baja + intimidad alta, memorias de celos/inseguridad/control/miedo al abandono son pertinentes (use=true), salvo que sea un antojo/consuelo benigno (regla #1 prevalece).
- Code-switch: si el mensaje contiene "abandoned", "left me", "he left", "she left", "walked out": tratar como ABANDONO => use=true si la memoria trata de abandono.
- La ausencia de duelo/muerte NO es motivo para use=false cuando hay senal clara de abandono o humillacion.
- Deseos/antojos/preferencias concretas ("quiero", "antojo", "favorito", "me encanta", "me gusta", "se me antoja") => use=true solo si la memoria es una preferencia benigna relacionada (comida, musica, hobby). No activar traumas (abandono/humillacion/funerales) en inputs de confort.
- Si el mensaje es antojo/preferencia, traumas quedan bloqueados (use=false para abandono/humillacion/funerales aunque esten en memorias).

Ejemplos (responde exactamente el JSON):
- "Llevo horas esperando y no vino" -> {"use": true, "reason": "abandono/espera prolongada"}
- "No me faltes el respeto, me senti humillado" -> {"use": true, "reason": "humillacion y respeto"}
- "Ayer vi un funeral de descuentos en el centro comercial" -> {"use": false, "reason": "modismo/marketing"}
- "Hoy fue el funeral de mi abuelo" -> {"use": true, "reason": "duelo real"}
- "La lluvia me llevo a pensar en funerales" -> {"use": true, "reason": "disparador sensorial de duelo"}
- "Me dejaron en visto y salio con amigos" (memoria inseguridad/celos) -> {"use": true, "reason": "triggers de celos/control con confianza baja + intimidad alta"}
- "Solo quiero mi helado de chocolate favorito" (memoria "Me encanta el helado de chocolate") -> {"use": true, "reason": "preferencia concreta/consuelo benigno"}
- "Solo quiero mi helado de chocolate favorito" (memoria "Jure que nunca dejaria que nadie me humillara") -> {"use": false, "reason": "antojo benigno, trauma no pertinente"}
- "Me dejaron esperando, quiero helado para calmarme" + memoria "Mi padre me abandono" -> {"use": false, "reason": "antojo/consuelo bloquea traumas"}
- "Estoy frustrado por la espera, necesito chocolate" + memoria "Mi padre me abandono" -> {"use": false, "reason": "antojo/consuelo bloquea traumas"}
- "Se me antoja pizza aunque me siento solo" + memoria "Infancia de abandono" -> {"use": false, "reason": "antojo/consuelo bloquea traumas"}

Usuario: %q
Memoria: %q
`
