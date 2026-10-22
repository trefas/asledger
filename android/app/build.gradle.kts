import java.util.Properties
import org.gradle.api.tasks.testing.Test
import org.gradle.testing.jacoco.plugins.JacocoTaskExtension
import org.gradle.testing.jacoco.tasks.JacocoCoverageVerification
import org.gradle.testing.jacoco.tasks.JacocoReport
import org.jetbrains.kotlin.gradle.dsl.JvmTarget

// Модуль приложения: один модуль :app, без DI-фреймворка.
plugins {
    alias(libs.plugins.android.application)
    alias(libs.plugins.kotlin.android)
    alias(libs.plugins.kotlin.compose)
    alias(libs.plugins.kotlin.serialization)
    alias(libs.plugins.ktlint)
    alias(libs.plugins.ksp)
    alias(libs.plugins.room)
    jacoco
}

// Релизный ключ читается из окружения CI или локального keystore.properties.
// Сам ключ и пароли в репозиторий не попадают.
val keystoreProperties = Properties().apply {
    val file = rootProject.file("keystore.properties")
    if (file.isFile) {
        file.inputStream().use { load(it) }
    }
}

fun signingValue(environment: String, property: String): String? =
    System.getenv(environment)?.takeIf { it.isNotBlank() }
        ?: keystoreProperties.getProperty(property)?.takeIf { it.isNotBlank() }

val releaseStorePath = signingValue("ANDROID_KEYSTORE_PATH", "storeFile")
val releaseStorePassword = signingValue("ANDROID_KEYSTORE_PASSWORD", "storePassword")
val releaseKeyAlias = signingValue("ANDROID_KEY_ALIAS", "keyAlias")
val releaseKeyPassword = signingValue("ANDROID_KEY_PASSWORD", "keyPassword")
val hasReleaseSigning = releaseStorePath != null &&
    releaseStorePassword != null &&
    releaseKeyAlias != null &&
    releaseKeyPassword != null &&
    rootProject.file(releaseStorePath).isFile

android {
    namespace = "com.trefas.asledger"
    compileSdk = 36

    defaultConfig {
        applicationId = "com.trefas.asledger"
        // Android 14 (API 34) и новее — требование эпика.
        minSdk = 34
        targetSdk = 36
        // Тег v1.2.3 даёт versionName 1.2.3 и versionCode 10203.
        versionCode = System.getenv("ANDROID_VERSION_CODE")?.toIntOrNull() ?: 10_000
        versionName = System.getenv("ANDROID_VERSION_NAME")
            ?.takeIf { it.isNotBlank() }
            ?: "1.0.0"

        testInstrumentationRunner = "androidx.test.runner.AndroidJUnitRunner"
        vectorDrawables {
            useSupportLibrary = true
        }

        // Таймауты API вынесены в BuildConfig, чтобы debug-сборка могла
        // подсказывать ошибки быстрее.
        buildConfigField("long", "API_TIMEOUT_SECONDS", "20L")
        buildConfigField("long", "API_CONNECT_TIMEOUT_SECONDS", "10L")
    }

    signingConfigs {
        if (hasReleaseSigning) {
            create("release") {
                storeFile = rootProject.file(requireNotNull(releaseStorePath))
                storePassword = releaseStorePassword
                keyAlias = releaseKeyAlias
                keyPassword = releaseKeyPassword
                // minSdk 34: JAR-подпись v1 не нужна. Оставляем v2 для
                // совместимости и добавляем v3, чтобы APK принимали
                // установщики, которым недостаточно только v2.
                enableV1Signing = false
                enableV2Signing = true
                enableV3Signing = true
                enableV4Signing = false
            }
        }
    }

    buildTypes {
        debug {
            applicationIdSuffix = ".debug"
            versionNameSuffix = "-debug"
            isMinifyEnabled = false
            // Нужно для локальной проверки покрытия слоя данных в CI.
            enableUnitTestCoverage = true
        }
        release {
            isMinifyEnabled = true
            isShrinkResources = true
            isDebuggable = false
            proguardFiles(
                getDefaultProguardFile("proguard-android-optimize.txt"),
                "proguard-rules.pro"
            )
            if (hasReleaseSigning) {
                signingConfig = signingConfigs.getByName("release")
            } else {
                // Локальная проверка release-сборки без ключа. Такой APK
                // нельзя публиковать: обновление поверх него не встанет.
                signingConfig = signingConfigs.getByName("debug")
                logger.warn(
                    "Release APK подписан debug-ключом: задайте ANDROID_KEYSTORE_PATH " +
                        "и пароли или android/keystore.properties."
                )
            }
        }
    }

    buildFeatures {
        compose = true
        buildConfig = true
    }

    compileOptions {
        sourceCompatibility = JavaVersion.VERSION_17
        targetCompatibility = JavaVersion.VERSION_17
    }

    packaging {
        resources {
            excludes += "/META-INF/{AL2.0,LGPL2.1}"
        }
    }

    lint {
        abortOnError = true
        warningsAsErrors = false
        // Новые версии библиотек проверяет Renovate/CI, а не блокирующая сборка.
        disable += "GradleDependency"
    }

    testOptions {
        unitTests {
            // Robolectric + Compose UI-тест навигации на JVM, без эмулятора.
            isIncludeAndroidResources = true
        }
    }
}

// Схема Room экспортируется в репозиторий для проверки миграций.
room {
    schemaDirectory("$projectDir/schemas")
}

kotlin {
    compilerOptions {
        jvmTarget.set(JvmTarget.JVM_17)
    }
}

// Стиль Kotlin проверяется ktlint в составе android-check и CI.
ktlint {
    version.set("1.5.0")
    android.set(true)
    outputToConsole.set(true)
    ignoreFailures.set(false)
}

dependencies {
    implementation(libs.androidx.core.ktx)
    implementation(libs.androidx.lifecycle.runtime.ktx)
    implementation(libs.androidx.lifecycle.runtime.compose)
    implementation(libs.androidx.activity.compose)

    implementation(platform(libs.androidx.compose.bom))
    implementation(libs.androidx.compose.ui)
    implementation(libs.androidx.compose.ui.graphics)
    implementation(libs.androidx.compose.ui.tooling.preview)
    implementation(libs.androidx.compose.material3)
    implementation(libs.androidx.navigation.compose)

    implementation(libs.androidx.datastore.preferences)
    implementation(libs.androidx.room.runtime)
    implementation(libs.androidx.room.ktx)
    implementation(libs.androidx.lifecycle.viewmodel.compose)
    implementation(libs.androidx.work.runtime)
    ksp(libs.androidx.room.compiler)
    implementation(libs.kotlinx.coroutines.android)
    implementation(libs.kotlinx.serialization.json)

    implementation(libs.ktor.client.core)
    implementation(libs.ktor.client.okhttp)
    implementation(libs.ktor.client.content.negotiation)
    implementation(libs.ktor.serialization.kotlinx.json)

    testImplementation(libs.junit)
    testImplementation(libs.kotlinx.coroutines.test)
    testImplementation(libs.ktor.client.mock)
    testImplementation(platform(libs.androidx.compose.bom))
    testImplementation(libs.androidx.compose.ui.test.junit4)
    testImplementation(libs.androidx.compose.ui.test.manifest)
    testImplementation(libs.robolectric)
    testImplementation(libs.androidx.room.testing)

    androidTestImplementation(libs.androidx.junit)
    androidTestImplementation(libs.androidx.espresso.core)
    androidTestImplementation(platform(libs.androidx.compose.bom))
    androidTestImplementation(libs.androidx.compose.ui.test.junit4)

    debugImplementation(libs.androidx.compose.ui.tooling)
    debugImplementation(libs.androidx.compose.ui.test.manifest)
}

// Покрытие слоя данных. Keystore и сборочный контейнер не входят в расчёт:
// AndroidKeyStore недоступен в Robolectric, а AppContainer — только проводка
// зависимостей. Остальные пакеты data/ проверяются unit-тестами.
jacoco {
    toolVersion = "0.8.13"
}

val dataCoverageClasses = fileTree(layout.buildDirectory.dir("tmp/kotlin-classes/debug")) {
    include("com/trefas/asledger/data/**")
    exclude(
        "com/trefas/asledger/data/AppContainer*",
        "com/trefas/asledger/data/security/**",
        "**/*_Impl*",
        "**/*_Factory*",
        "**/*Database_Impl*"
    )
}

val dataCoverageSources = fileTree("src/main/java") {
    include("com/trefas/asledger/data/**")
    exclude(
        "com/trefas/asledger/data/AppContainer*",
        "com/trefas/asledger/data/security/**"
    )
}

val dataCoverageExec = layout.buildDirectory.file(
    "outputs/unit_test_code_coverage/debugUnitTest/testDebugUnitTest.exec"
)

// Robolectric загружает классы приложения через AndroidSandbox. У них нет
// code source, поэтому JaCoCo по умолчанию их пропускает.
tasks.withType<Test>().configureEach {
    extensions.configure<JacocoTaskExtension> {
        isEnabled = true
        destinationFile = dataCoverageExec.get().asFile
        setIncludeNoLocationClasses(true)
        // Без этого агент пытается инструментировать сгенерированные JDK
        // классы без code source и падает. Нам нужен только слой data/.
        includes = listOf("com.trefas.asledger.data.*")
    }
}

tasks.register<JacocoReport>("jacocoDataReport") {
    dependsOn("testDebugUnitTest")
    executionData.setFrom(dataCoverageExec)
    classDirectories.setFrom(dataCoverageClasses)
    sourceDirectories.setFrom(dataCoverageSources)
    reports {
        xml.required.set(true)
        html.required.set(true)
    }
}

tasks.register<JacocoCoverageVerification>("verifyDataCoverage") {
    dependsOn("testDebugUnitTest")
    executionData.setFrom(dataCoverageExec)
    classDirectories.setFrom(dataCoverageClasses)
    sourceDirectories.setFrom(dataCoverageSources)
    violationRules {
        rule {
            limit {
                counter = "LINE"
                value = "COVEREDRATIO"
                minimum = "0.70".toBigDecimal()
            }
        }
    }
}
