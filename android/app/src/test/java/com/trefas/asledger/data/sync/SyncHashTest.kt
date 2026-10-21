package com.trefas.asledger.data.sync

import com.trefas.asledger.data.model.SyncCommit
import org.junit.Assert.assertEquals
import org.junit.Assert.assertNull
import org.junit.Test

/** Хэш-цепочка считается по точным строкам ops_json, без сериализации DTO. */
class SyncHashTest {
    @Test
    fun `звено совпадает с SHA-256 Go-сервера`() {
        assertEquals(
            "6b86b273ff34fce19d6b804eff5a3f5747ada4eaa22f1d49c01e52ddb7875b4b",
            chainHash("", "1")
        )
        assertEquals(
            "9f86d081884c7d659a2feaa0c55ad015a3bf4f1b2b0b822cd15d6c15b0f00a08",
            chainHash("", "test")
        )
    }

    @Test
    fun `подмена ops_json ломает цепочку`() {
        val first = SyncCommit(
            id = 1,
            clientId = "dev",
            actorId = 1,
            baseSeq = 0,
            opsJson = """{"commit_id":"c1","operations":[]}""",
            resultSeq = 1,
            hash = chainHash("", """{"commit_id":"c1","operations":[]}"""),
            warnings = emptyList(),
            createdAt = 0
        )
        val second = first.copy(
            id = 2,
            opsJson = """{"commit_id":"c2","operations":[]}""",
            resultSeq = 2,
            hash = chainHash(first.hash, """{"commit_id":"c2","operations":[]}""")
        )

        assertEquals(second.hash, verifyChain("", listOf(first, second)))
        assertNull(verifyChain("", listOf(first.copy(opsJson = """{"подмена":true}"""), second)))
        assertNull(verifyChain("другой-хеш", listOf(first, second)))
    }
}
