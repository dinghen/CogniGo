<template>
  <div class="ai-chat-container">
    <!-- 左侧会话列表 -->
    <div class="session-list">
      <div class="session-list-header">
        <span>会话列表</span>
        <button class="new-chat-btn" @click="createNewSession">＋ 新聊天</button>
      </div>
      <ul class="session-list-ul">
        <li
          v-for="session in sessions"
          :key="session.id"
          :class="['session-item', { active: currentSessionId === session.id }]"
          @click="switchSession(session.id)"
        >
          {{ session.name || `会话 ${session.id}` }}
        </li>
      </ul>
    </div>

    <!-- 右侧聊天区域 -->
    <div class="chat-section">
      <div class="top-bar">
        <button class="back-btn" @click="$router.push('/menu')">← 返回</button>
        <button class="sync-btn" @click="syncHistory" :disabled="!currentSessionId || tempSession">同步历史数据</button>
        <button class="settings-btn" @click="$router.push('/settings')">配置能力</button>
        <label for="streamingMode" style="margin-left: 20px;">
          <input type="checkbox" id="streamingMode" v-model="isStreaming" />
          流式响应
        </label>
        <button class="upload-btn" @click="triggerFileUpload" :disabled="uploading">📎 上传文档(.md/.txt)</button>
        <input
          ref="fileInput"
          type="file"
          accept=".md,.txt,text/markdown,text/plain"
          style="display: none"
          @change="handleFileUpload"
        />
      </div>

      <section class="capability-panel" aria-label="聊天能力选择">
        <div class="capability-field">
          <label for="chatProvider">聊天模型</label>
          <select id="chatProvider" v-model="selectedChatProviderId" class="capability-select" :disabled="!chatProviders.length" @change="rememberCurrentSessionCapabilities">
            <option value="">使用默认连接</option>
            <option v-for="provider in chatProviders" :key="provider.id" :value="String(provider.id)">
              {{ provider.name }} · {{ provider.model }}{{ provider.status && provider.status !== 'ready' ? `（${provider.status}）` : '' }}
            </option>
          </select>
          <small v-if="!chatProviders.length" class="capability-hint">还没有配置聊天模型，请先前往设置。</small>
        </div>
        <div class="capability-field">
          <label for="embeddingProvider">知识检索模型</label>
          <select id="embeddingProvider" v-model="selectedEmbeddingProviderId" class="capability-select" :disabled="!embeddingProviders.length" @change="rememberCurrentSessionCapabilities">
            <option value="">使用默认 Embedding 连接</option>
            <option v-for="provider in embeddingProviders" :key="provider.id" :value="String(provider.id)">
              {{ provider.name }} · {{ provider.model }}{{ provider.status && provider.status !== 'ready' ? `（${provider.status}）` : '' }}
            </option>
          </select>
          <small v-if="!embeddingProviders.length" class="capability-hint">没有配置 Embedding 模型时仍可进行普通对话。</small>
        </div>
        <div class="capability-field capability-field-wide">
          <label for="mcpServers">MCP 服务</label>
          <select id="mcpServers" v-model="selectedMcpServerIds" multiple class="capability-select" :disabled="!mcpServers.length" @change="handleMcpSelectionChange">
            <option v-for="server in mcpServers" :key="server.id" :value="String(server.id)" :disabled="server.enabled === false">
              {{ server.name }}{{ server.status && server.status !== 'healthy' ? `（${server.status}）` : '' }}
            </option>
          </select>
          <small v-if="!mcpServers.length" class="capability-hint">还没有 MCP 服务；不选择服务时不会建立 MCP 连接。</small>
          <small v-else-if="!selectedMcpServers.length" class="capability-hint">当前未选择 MCP 服务，本次请求不会连接 MCP。</small>
        </div>
        <div v-if="selectedMcpServers.length" class="capability-tools capability-field-wide">
          <span class="capability-label">可用工具（未选择工具时包含所选服务的全部工具）</span>
          <details v-for="server in selectedMcpServers" :key="server.id" open class="tool-group">
            <summary>{{ server.name }}（{{ (server.tools || []).length }}）</summary>
            <label v-for="tool in server.tools || []" :key="tool.id" class="tool-option">
              <input v-model="selectedMcpToolIds" type="checkbox" :value="String(tool.id)" :disabled="tool.enabled === false" @change="rememberCurrentSessionCapabilities" />
              <span>{{ tool.name }}</span>
            </label>
            <small v-if="!(server.tools || []).length" class="capability-hint">尚未发现工具，请在设置中测试并发现。</small>
          </details>
        </div>
        <div class="knowledge-status" :class="{ 'knowledge-error': knowledgeError }">
          <span v-if="loadingKnowledge">正在检查知识库状态…</span>
          <span v-else-if="knowledgeError">{{ knowledgeError }} <button type="button" class="inline-action" @click="loadKnowledgeFiles">重新加载</button></span>
          <span v-else-if="!knowledgeFiles.length">还没有知识文件，当前仍可进行普通对话。</span>
          <span v-else-if="!readyKnowledgeFiles.length">知识库尚不可检索；{{ needsRebuildCount ? '请在设置中重建索引。' : '文件正在处理，完成后会自动用于检索。' }}当前仍可进行普通对话。</span>
          <span v-else>知识库已就绪：{{ readyKnowledgeFiles.length }} 个文件可用于检索。<span v-if="needsRebuildCount">另有 {{ needsRebuildCount }} 个文件需要重建索引。</span></span>
        </div>
        <p v-if="selectedChatProvider && selectedChatProvider.status && selectedChatProvider.status !== 'ready'" class="capability-hint">
          所选聊天模型状态为 {{ selectedChatProvider.status }}；请在设置中测试连接后重试。
        </p>
        <p v-if="selectedEmbeddingProvider && selectedEmbeddingProvider.status !== 'ready'" class="capability-hint">
          所选检索模型状态为 {{ selectedEmbeddingProvider.status || '未验证' }}；如需使用知识检索，请在设置中测试连接并重建索引。
        </p>
        <p v-if="providerError || mcpError || uploadError" class="capability-error" role="alert">
          {{ [providerError, mcpError, uploadError].filter(Boolean).join(' ') }}
          <button type="button" class="inline-action" @click="refreshCapabilities">重新加载状态</button>
        </p>
        <p v-if="chatError" class="capability-error" role="alert">{{ chatError }}</p>
      </section>

      <div class="chat-messages" ref="messagesRef">
        <div
          v-for="(message, index) in currentMessages"
          :key="index"
          :class="['message', message.role === 'user' ? 'user-message' : 'ai-message']"
        >
          <div class="message-header">
            <b>{{ message.role === 'user' ? '你' : 'AI' }}:</b>
            <button v-if="message.role === 'assistant'" class="tts-btn" @click="playTTS(message.content)">🔊</button>
            <span v-if="message.meta && message.meta.status === 'streaming'" class="streaming-indicator"> ··</span>
          </div>
          <div class="message-content" v-html="renderMarkdown(message.content)"></div>
        </div>
      </div>

      <div class="chat-input">
        <textarea
          v-model="inputMessage"
          placeholder="请输入你的问题..."
          @keydown.enter.exact.prevent="sendMessage"
          :disabled="loading"
          ref="messageInput"
          rows="1"
        ></textarea>
        <button
          type="button"
          :disabled="!inputMessage.trim() || loading"
          @click="sendMessage"
          class="send-btn"
        >
          {{ loading ? '发送中...' : '发送' }}
        </button>
      </div>
    </div>
  </div>
</template>

<script>


import { ref, nextTick, computed, onMounted } from 'vue'
import { ElMessage } from 'element-plus'
import api from '../utils/api'

export default {
  name: 'AIChat',
  setup() {

    const sessions = ref({})
    const currentSessionId = ref(null)
    const tempSession = ref(false)
    const currentMessages = ref([])
    const inputMessage = ref('')
    const loading = ref(false)
    const messagesRef = ref(null)
    const messageInput = ref(null)
    const providers = ref([])
    const chatProviders = computed(() => providers.value.filter(provider => provider.kind === 'chat'))
    const embeddingProviders = computed(() => providers.value.filter(provider => provider.kind === 'embedding'))
    const selectedChatProviderId = ref('')
    const selectedEmbeddingProviderId = ref('')
    const mcpServers = ref([])
    const mcpServersLoaded = ref(false)
    const selectedMcpServerIds = ref([])
    const selectedMcpToolIds = ref([])
    const selectedMcpServers = computed(() => mcpServers.value.filter(server => server.enabled !== false && selectedMcpServerIds.value.includes(String(server.id))))
    const knowledgeFiles = ref([])
    const readyKnowledgeFiles = computed(() => knowledgeFiles.value.filter(file => file.status === 'ready'))
    const needsRebuildCount = computed(() => knowledgeFiles.value.filter(file => ['stale', 'failed', 'rebuild_required'].includes(file.status)).length)
    const selectedEmbeddingProvider = computed(() => embeddingProviders.value.find(provider => String(provider.id) === selectedEmbeddingProviderId.value))
    const selectedChatProvider = computed(() => chatProviders.value.find(provider => String(provider.id) === selectedChatProviderId.value))
    const providerError = ref('')
    const mcpError = ref('')
    const knowledgeError = ref('')
    const chatError = ref('')
    const uploadError = ref('')
    const loadingKnowledge = ref(false)
    const sessionCapabilities = ref({})
    let providersInitialized = false
    const isStreaming = ref(false)
    const uploading = ref(false)
    const fileInput = ref(null)


    const renderMarkdown = (text) => {
      if (!text && text !== '') return ''
      return String(text)
        .replace(/\*\*(.*?)\*\*/g, '<strong>$1</strong>')
        .replace(/\*(.*?)\*/g, '<em>$1</em>')
        .replace(/`(.*?)`/g, '<code>$1</code>')
        .replace(/\n/g, '<br>')
    }

    const playTTS = async (text) => {
      try {
        // 创建TTS任务
        const createResponse = await api.post('/AI/chat/tts', { text })
        if (createResponse.data && createResponse.data.status_code === 1000 && createResponse.data.task_id) {
          const taskId = createResponse.data.task_id
          
          // 先等待5秒钟再开始轮询
          await new Promise(resolve => setTimeout(resolve, 5000))
          
          // 轮询查询任务结果
          const maxAttempts = 30
          const pollInterval = 2000
          let attempts = 0
          
          const pollResult = async () => {
            const queryResponse = await api.get('/AI/chat/tts/query', { params: { task_id: taskId } })
            
            if (queryResponse.data && queryResponse.data.status_code === 1000) {
              const taskStatus = queryResponse.data.task_status
                
              if (taskStatus === 'Success' && queryResponse.data.task_result) {
                // 任务完成，播放音频
                // 后端返回的 task_result 是直接的 URL 字符串
                const audio = new Audio(queryResponse.data.task_result)
                audio.play()
                return true
              } else if (taskStatus === 'Running' ||taskStatus === 'Created' ) {
                // 任务进行中，继续轮询
                attempts++
                if (attempts < maxAttempts) {
                  await new Promise(resolve => setTimeout(resolve, pollInterval))
                  return await pollResult()
                } else {
                  ElMessage.error('语音合成超时')
                  return true
                }
              } else {
                // 其他状态（如失败）
                ElMessage.error('语音合成失败')
                return true
              }
            }
            
            attempts++
            if (attempts < maxAttempts) {
              await new Promise(resolve => setTimeout(resolve, pollInterval))
              return await pollResult()
            } else {
              ElMessage.error('语音合成超时')
              return true
            }
          }
          
          await pollResult()
        } else {
          ElMessage.error('无法创建语音合成任务')
        }
      } catch (error) {
        console.error('TTS error:', error)
        ElMessage.error('请求语音接口失败')
      }
    }

    const capabilitySnapshot = () => ({
      chatProviderId: selectedChatProviderId.value,
      embeddingProviderId: selectedEmbeddingProviderId.value,
      mcpServerIds: [...selectedMcpServerIds.value],
      mcpToolIds: [...selectedMcpToolIds.value]
    })

    const rememberCurrentSessionCapabilities = () => {
      if (!tempSession.value && currentSessionId.value) {
        sessionCapabilities.value[currentSessionId.value] = capabilitySnapshot()
      }
    }

    const restoreSessionCapabilities = sessionId => {
      const saved = sessionCapabilities.value[sessionId]
      selectedChatProviderId.value = saved?.chatProviderId || ''
      selectedEmbeddingProviderId.value = saved?.embeddingProviderId || ''
      selectedMcpServerIds.value = [...(saved?.mcpServerIds || [])]
      selectedMcpToolIds.value = [...(saved?.mcpToolIds || [])]
      if (mcpServersLoaded.value) handleMcpSelectionChange()
    }

    const getCapabilityPayload = () => {
      const payload = {}
      if (!providerError.value) {
        payload.chatProviderId = selectedChatProvider.value ? Number(selectedChatProvider.value.id) : 0
        payload.embeddingProviderId = selectedEmbeddingProvider.value ? Number(selectedEmbeddingProvider.value.id) : 0
      }
      if (!mcpError.value && mcpServersLoaded.value) {
        payload.mcpServerIds = selectedMcpServers.value.map(server => Number(server.id))
        payload.mcpToolIds = selectedMcpToolIds.value.filter(id => selectedMcpServers.value.some(server => (server.tools || []).some(tool => String(tool.id) === id && tool.enabled !== false))).map(Number)
      }
      return payload
    }

    const errorMessage = (error, fallback) => {
      const data = error?.response?.data
      return data?.status_msg || data?.message || data?.error || (error?.message && error.message !== 'Failed to fetch' ? error.message : fallback)
    }

    const loadSessions = async () => {
      try {
        const response = await api.get('/AI/chat/sessions')
        if (response.data && response.data.status_code === 1000 && Array.isArray(response.data.sessions)) {
          const sessionMap = {}
          response.data.sessions.forEach(s => {
            const sid = String(s.sessionId)
            sessionMap[sid] = {
              id: sid,
              name: s.name || `会话 ${sid}`,
              messages: [] // lazy load
            }
            if (Array.isArray(s.mcpServerIds) || s.chatProviderId || s.embeddingProviderId) {
              sessionCapabilities.value[sid] = {
                chatProviderId: s.chatProviderId ? String(s.chatProviderId) : '',
                embeddingProviderId: s.embeddingProviderId ? String(s.embeddingProviderId) : '',
                mcpServerIds: (s.mcpServerIds || []).map(String),
                mcpToolIds: (s.mcpToolIds || []).map(String)
              }
            }
          })
          sessions.value = sessionMap
        }
      } catch (error) {
        console.error('Load sessions error:', error)
      }
    }

    const loadProviders = async () => {
      try {
        const response = await api.get('/settings/providers')
        if (response.data?.status_code !== 1000) throw new Error(response.data?.status_msg || '模型连接加载失败')
        providers.value = response.data.providers || []
        if (!providersInitialized) {
          const defaultChat = chatProviders.value.find(provider => provider.is_default) || (chatProviders.value.length === 1 ? chatProviders.value[0] : null)
          const defaultEmbedding = embeddingProviders.value.find(provider => provider.is_default) || (embeddingProviders.value.length === 1 ? embeddingProviders.value[0] : null)
          selectedChatProviderId.value = defaultChat ? String(defaultChat.id) : ''
          selectedEmbeddingProviderId.value = defaultEmbedding ? String(defaultEmbedding.id) : ''
          providersInitialized = true
        } else {
          if (selectedChatProviderId.value && !chatProviders.value.some(provider => String(provider.id) === selectedChatProviderId.value)) selectedChatProviderId.value = ''
          if (selectedEmbeddingProviderId.value && !embeddingProviders.value.some(provider => String(provider.id) === selectedEmbeddingProviderId.value)) selectedEmbeddingProviderId.value = ''
        }
        providerError.value = ''
      } catch (error) {
        console.error('Load providers error:', error)
        providerError.value = '模型连接加载失败，请检查配置中心后重试。'
      }
    }

    const loadMcpServers = async () => {
      try {
        const response = await api.get('/settings/mcp-servers')
        if (response.data?.status_code !== 1000) throw new Error(response.data?.status_msg || 'MCP 服务加载失败')
        mcpServers.value = response.data.servers || []
        mcpServersLoaded.value = true
        const serverIds = new Set(mcpServers.value.filter(server => server.enabled !== false).map(server => String(server.id)))
        selectedMcpServerIds.value = selectedMcpServerIds.value.filter(id => serverIds.has(id))
        handleMcpSelectionChange()
        mcpError.value = ''
      } catch (error) {
        console.error('Load MCP servers error:', error)
        mcpError.value = 'MCP 服务和工具目录加载失败，请在设置中测试服务后重试。'
      }
    }

    const loadKnowledgeFiles = async () => {
      loadingKnowledge.value = true
      try {
        const response = await api.get('/file/files')
        if (response.data?.status_code !== 1000) throw new Error(response.data?.status_msg || '知识文件状态加载失败')
        knowledgeFiles.value = response.data.files || []
        knowledgeError.value = ''
      } catch (error) {
        console.error('Load knowledge files error:', error)
        knowledgeError.value = '知识文件状态加载失败。'
      } finally {
        loadingKnowledge.value = false
      }
    }

    const refreshCapabilities = () => Promise.all([loadProviders(), loadMcpServers(), loadKnowledgeFiles()])

    const handleMcpSelectionChange = () => {
      if (!mcpServersLoaded.value) return
      const availableIds = new Set(selectedMcpServers.value.flatMap(server => (server.tools || []).filter(tool => tool.enabled !== false).map(tool => String(tool.id))))
      selectedMcpToolIds.value = selectedMcpToolIds.value.filter(id => availableIds.has(id))
      rememberCurrentSessionCapabilities()
    }

    const createNewSession = () => {
      currentSessionId.value = 'temp'
      tempSession.value = true
      currentMessages.value = []
      // focus input
      nextTick(() => {
        if (messageInput.value) messageInput.value.focus()
      })
    }

    const switchSession = async (sessionId) => {
      if (!sessionId) return
      rememberCurrentSessionCapabilities()
      currentSessionId.value = String(sessionId)
      tempSession.value = false
      restoreSessionCapabilities(currentSessionId.value)

      // lazy load history if not present
      if (!sessions.value[sessionId].messages || sessions.value[sessionId].messages.length === 0) {
        try {
          const response = await api.post('/AI/chat/history', { sessionId: currentSessionId.value })
          if (response.data && response.data.status_code === 1000 && Array.isArray(response.data.history)) {
            const messages = response.data.history.map(item => ({
              role: item.is_user ? 'user' : 'assistant',
              content: item.content
            }))
            sessions.value[sessionId].messages = messages
          }
        } catch (err) {
          console.error('Load history error:', err)
        }
      }


      currentMessages.value = [...(sessions.value[sessionId].messages || [])]
      await nextTick()
      scrollToBottom()
    }

    const syncHistory = async () => {
      if (!currentSessionId.value || tempSession.value) {
        ElMessage.warning('请选择已有会话进行同步')
        return
      }
      try {
        const response = await api.post('/AI/chat/history', { sessionId: currentSessionId.value })
        if (response.data && response.data.status_code === 1000 && Array.isArray(response.data.history)) {
          const messages = response.data.history.map(item => ({
            role: item.is_user ? 'user' : 'assistant',
            content: item.content
          }))
          sessions.value[currentSessionId.value].messages = messages
          currentMessages.value = [...messages]
          await nextTick()
          scrollToBottom()
        } else {
          ElMessage.error('无法获取历史数据')
        }
      } catch (err) {
        console.error('Sync history error:', err)
        ElMessage.error('请求历史数据失败')
      }
    }


    const sendMessage = async () => {
      if (!inputMessage.value || !inputMessage.value.trim()) {
        ElMessage.warning('请输入消息内容')
        return
      }

      chatError.value = ''
      rememberCurrentSessionCapabilities()

      const userMessage = {
        role: 'user',
        content: inputMessage.value
      }
      const currentInput = inputMessage.value
      inputMessage.value = ''


      currentMessages.value.push(userMessage)
      await nextTick()
      scrollToBottom()

      try {
        loading.value = true
        if (isStreaming.value) {

          await handleStreaming(currentInput)
        } else {

          await handleNormal(currentInput)
        }
      } catch (err) {
        console.error('Send message error:', err)
        chatError.value = errorMessage(err, '发送失败，请检查当前能力配置后重试。')
        ElMessage.error(chatError.value)

        if (!tempSession.value && currentSessionId.value && sessions.value[currentSessionId.value] && sessions.value[currentSessionId.value].messages) {

          const sessionArr = sessions.value[currentSessionId.value].messages
          if (sessionArr && sessionArr.length) sessionArr.pop()
        }
        currentMessages.value.pop()
      } finally {
        if (!isStreaming.value) {
          loading.value = false
        }
        await nextTick()
        scrollToBottom()
      }
    }


    async function handleStreaming(question) {

      const aiMessage = {
        role: 'assistant',
        content: '',
        meta: { status: 'streaming' } // mark streaming
      }


      const aiMessageIndex = currentMessages.value.length
      currentMessages.value.push(aiMessage)

      if (!tempSession.value && currentSessionId.value && sessions.value[currentSessionId.value]) {
        if (!sessions.value[currentSessionId.value].messages) sessions.value[currentSessionId.value].messages = []
        sessions.value[currentSessionId.value].messages.push({ role: 'user', content: question })
        sessions.value[currentSessionId.value].messages.push({ role: 'assistant', content: '' })
      }


      const url = tempSession.value
        ? '/api/AI/chat/send-stream-new-session'  
        : '/api/AI/chat/send-stream'           

      const headers = {
        'Content-Type': 'application/json',
        'Authorization': `Bearer ${localStorage.getItem('token') || ''}`
      }

      const body = {
        question,
        ...getCapabilityPayload(),
        ...(tempSession.value ? {} : { sessionId: currentSessionId.value })
      }

      try {
        // 创建 fetch 连接读取 SSE 流
        const response = await fetch(url, {
          method: 'POST',
          headers,
          body: JSON.stringify(body)
        })

        if (!response.ok) {
          const payload = await response.json().catch(() => null)
          throw new Error(errorMessage({ response: { data: payload } }, `聊天请求失败（${response.status}）`))
        }
        if (!(response.headers.get('content-type') || '').includes('text/event-stream') || !response.body) {
          const payload = await response.json().catch(() => null)
          throw new Error(errorMessage({ response: { data: payload } }, '聊天服务未返回流式响应，请检查服务状态后重试。'))
        }

        const reader = response.body.getReader()
        const decoder = new TextDecoder()
        let buffer = ''
        let eventType = 'message'

        // 读取流数据
        // eslint-disable-next-line no-constant-condition
        while (true) {
          const { done, value } = await reader.read()
          if (done) break

          const chunk = decoder.decode(value, { stream: true })
          buffer += chunk

          // 按行分割
          const lines = buffer.split('\n')
          buffer = lines.pop() || '' // 保留未完成的行

          for (const line of lines) {
            const trimmedLine = line.trim()
            if (!trimmedLine) continue

            if (trimmedLine.startsWith('event:')) {
              eventType = trimmedLine.slice(6).trim()
              continue
            }

            // 处理 SSE 格式：data: <content>
            if (trimmedLine.startsWith('data:')) {
              const data = trimmedLine.slice(5).trim()
              console.log('[SSE] Received:', data) // 调试日志

              if (data === '[DONE]') {
                // 流结束
                console.log('[SSE] Stream done')
                loading.value = false
                currentMessages.value[aiMessageIndex].meta = { status: 'done' }
                currentMessages.value = [...currentMessages.value]
              } else {
                let parsed = null
                try {
                  parsed = JSON.parse(data)
                } catch (_) {
                  // Plain text chunks are not JSON.
                }

                if (parsed && typeof parsed === 'object') {
                  if (parsed.sessionId) {
                    const newSid = String(parsed.sessionId)
                    console.log('[SSE] Session ID:', newSid)
                    if (tempSession.value) {
                      sessions.value[newSid] = {
                        id: newSid,
                        name: '新会话',
                        messages: [...currentMessages.value]
                      }
                      currentSessionId.value = newSid
                      tempSession.value = false
                      rememberCurrentSessionCapabilities()
                    }
                  } else if (eventType === 'error' || parsed.error || parsed.message || parsed.status_msg) {
                    throw new Error(parsed.status_msg || parsed.message || parsed.error || '聊天流请求失败。')
                  } else {
                    currentMessages.value[aiMessageIndex].content += data
                  }
                } else {
                  currentMessages.value[aiMessageIndex].content += data
                }
                console.log('[SSE] Content updated:', currentMessages.value[aiMessageIndex].content.length)
              }
              eventType = 'message'

              // 每收到一条数据就立即更新 DOM
              // 强制更新整个数组以触发响应式
              currentMessages.value = [...currentMessages.value]
              
              // 使用 requestAnimationFrame 强制浏览器重排
              await new Promise(resolve => {
                requestAnimationFrame(() => {
                  scrollToBottom()
                  resolve()
                })
              })
            }
          }
        }

        // 流读取完成后的处理
        loading.value = false
        currentMessages.value[aiMessageIndex].meta = { status: 'done' }
        currentMessages.value = [...currentMessages.value]
        chatError.value = ''

        // 同步到 sessions 存储
        if (!tempSession.value && currentSessionId.value && sessions.value[currentSessionId.value]) {
          const sessMsgs = sessions.value[currentSessionId.value].messages
          if (Array.isArray(sessMsgs) && sessMsgs.length) {
            const lastIndex = sessMsgs.length - 1
            if (sessMsgs[lastIndex] && sessMsgs[lastIndex].role === 'assistant') {
              sessMsgs[lastIndex].content = currentMessages.value[aiMessageIndex].content
            }
          }
        }
      } catch (err) {
        console.error('Stream error:', err)
        loading.value = false
        currentMessages.value[aiMessageIndex].meta = { status: 'error' }
        if (!currentMessages.value[aiMessageIndex].content) currentMessages.value.splice(aiMessageIndex, 1)
        const currentSession = sessions.value[currentSessionId.value]
        const sessionMessages = currentSession?.messages
        if (Array.isArray(sessionMessages) && sessionMessages.length && sessionMessages[sessionMessages.length - 1].role === 'assistant') {
          if (!sessionMessages[sessionMessages.length - 1].content) sessionMessages.pop()
          else sessionMessages[sessionMessages.length - 1].meta = { status: 'error' }
        }
        currentMessages.value = [...currentMessages.value]
        chatError.value = errorMessage(err, '流式传输出错，请检查当前能力配置后重试。')
        ElMessage.error(chatError.value)
      }
    }


    async function handleNormal(question) {
      if (tempSession.value) {

        const response = await api.post('/AI/chat/send-new-session', {
          question: question,
          ...getCapabilityPayload()
        })
        if (response.data && response.data.status_code === 1000) {
          const sessionId = String(response.data.sessionId)
          const aiMessage = {
            role: 'assistant',
            content: response.data.Information || ''
          }

          sessions.value[sessionId] = {
            id: sessionId,
            name: '新会话',
            messages: [ { role: 'user', content: question }, aiMessage ]
          }
          currentSessionId.value = sessionId
          tempSession.value = false
          rememberCurrentSessionCapabilities()
          currentMessages.value = [...sessions.value[sessionId].messages]
        } else {
          chatError.value = response.data?.status_msg || '创建会话失败，请检查聊天模型配置后重试。'
          ElMessage.error(chatError.value)

          currentMessages.value.pop()
        }
      } else {

        const sessionMsgs = sessions.value[currentSessionId.value].messages

        sessionMsgs.push({ role: 'user', content: question })

        const response = await api.post('/AI/chat/send', {
          question: question,
          sessionId: currentSessionId.value,
          ...getCapabilityPayload()
        })
        if (response.data && response.data.status_code === 1000) {
          const aiMessage = { role: 'assistant', content: response.data.Information || '' }
          sessionMsgs.push(aiMessage)
          currentMessages.value = [...sessionMsgs]
        } else {
          chatError.value = response.data?.status_msg || '发送失败，请检查当前能力配置后重试。'
          sessionMsgs.pop() // rollback
          currentMessages.value.pop()
          ElMessage.error(chatError.value)
        }
      }
    }


    const scrollToBottom = () => {
      if (messagesRef.value) {
        try {
          messagesRef.value.scrollTop = messagesRef.value.scrollHeight
        } catch (e) {
          // ignore
        }
      }
    }

    const triggerFileUpload = () => {
      if (fileInput.value) {
        fileInput.value.click()
      }
    }

    const handleFileUpload = async (event) => {
      const file = event.target.files[0]
      if (!file) return

      // 前端校验：只允许.md或.txt文件
      const fileName = file.name.toLowerCase()
      if (!fileName.endsWith('.md') && !fileName.endsWith('.txt')) {
        ElMessage.error('只允许上传 .md 或 .txt 文件')
        // 清空文件输入
        if (fileInput.value) {
          fileInput.value.value = ''
        }
        return
      }

      try {
        uploading.value = true
        uploadError.value = ''
        const formData = new FormData()
        formData.append('file', file)

        const response = await api.post('/file/upload', formData, {
          headers: {
            'Content-Type': 'multipart/form-data'
          }
        })

        if (response.data && response.data.status_code === 1000) {
          ElMessage.success(`文件上传成功`)
        } else {
          uploadError.value = response.data?.status_msg || '上传失败，请确认文件格式和索引服务状态。'
          ElMessage.error(uploadError.value)
        }
      } catch (error) {
        console.error('File upload error:', error)
        uploadError.value = errorMessage(error, '文件上传失败，请稍后重试。')
        ElMessage.error(uploadError.value)
      } finally {
        uploading.value = false
        await loadKnowledgeFiles()
        // 清空文件输入
        if (fileInput.value) {
          fileInput.value.value = ''
        }
      }
    }

    onMounted(() => {
      loadSessions()
      refreshCapabilities()
    })

    // expose to template
    return {
      sessions: computed(() => Object.values(sessions.value)),
      currentSessionId,
      tempSession,
      currentMessages,
      inputMessage,
      loading,
      messagesRef,
      messageInput,
      providers,
      chatProviders,
      embeddingProviders,
      selectedChatProviderId,
      selectedEmbeddingProviderId,
      mcpServers,
      selectedMcpServerIds,
      selectedMcpToolIds,
      selectedMcpServers,
      handleMcpSelectionChange,
      rememberCurrentSessionCapabilities,
      isStreaming,
      uploading,
      fileInput,
      knowledgeFiles,
      readyKnowledgeFiles,
      needsRebuildCount,
      selectedEmbeddingProvider,
      selectedChatProvider,
      providerError,
      mcpError,
      knowledgeError,
      chatError,
      uploadError,
      loadingKnowledge,
      loadKnowledgeFiles,
      refreshCapabilities,
      renderMarkdown,
      playTTS,
      createNewSession,
      switchSession,
      syncHistory,
      sendMessage,
      triggerFileUpload,
      handleFileUpload
    }
  }
}
</script>

<style scoped>
.ai-chat-container {
  height: 100vh;
  display: flex;
  background: #f3f5f7;
  position: relative;
  overflow: hidden;
  font-family: -apple-system, BlinkMacSystemFont, "Segoe UI", Roboto, "Helvetica Neue", Arial;
  color: #222;
}

.session-list {
  width: 280px;
  height: 100vh;
  overflow: hidden;
  display: flex;
  flex-direction: column;
  background: #fff;
  border-right: 1px solid rgba(0, 0, 0, 0.08);
  box-shadow: 2px 0 20px rgba(0, 0, 0, 0.08);
  position: relative;
  z-index: 2;
}

.session-list-header {
  padding: 20px;
  text-align: center;
  font-weight: 600;
  background: #f7f9fb;
  border-bottom: 1px solid rgba(0, 0, 0, 0.06);
  display: flex;
  flex-direction: column;
  gap: 12px;
  align-items: center;
}

.new-chat-btn {
  width: 100%;
  padding: 12px 0;
  cursor: pointer;
  background: #1677c8;
  color: white;
  border: none;
  border-radius: 12px;
  font-size: 14px;
  font-weight: 600;
  box-shadow: 0 2px 8px rgba(22, 119, 200, 0.2);
  transition: all 0.25s ease;
  position: relative;
  overflow: hidden;
}

.new-chat-btn:hover {
  background: #125f9f;
}

.session-list-ul {
  list-style: none;
  padding: 0;
  margin: 0;
  flex: 1;
  overflow-y: auto;
}

.session-item {
  padding: 15px 20px;
  cursor: pointer;
  border-bottom: 1px solid rgba(0, 0, 0, 0.03);
  transition: all 0.2s ease;
  position: relative;
  color: #2c3e50;
}

.session-item.active {
  background: #e8f2fb;
  color: #12558a;
  font-weight: 600;
  box-shadow: inset 3px 0 0 #1677c8;
}

.session-item:hover {
  background: #f2f7fb;
}

/* chat section */
.chat-section {
  flex: 1;
  display: flex;
  flex-direction: column;
  position: relative;
  z-index: 1;
  min-width: 0;
  min-height: 0;
  overflow: hidden;
}

.top-bar {
  background: #fff;
  color: #2c3e50;
  display: flex;
  align-items: center;
  padding: 12px 24px;
  box-shadow: 0 2px 14px rgba(0, 0, 0, 0.06);
  border-bottom: 1px solid rgba(0, 0, 0, 0.06);
  gap: 12px;
  flex-wrap: wrap;
}

.back-btn {
  background: rgba(255, 255, 255, 0.22);
  border: 1px solid rgba(0, 0, 0, 0.06);
  color: #2c3e50;
  padding: 8px 14px;
  border-radius: 10px;
  cursor: pointer;
  font-weight: 600;
  transition: all 0.2s ease;
  box-shadow: 0 2px 8px rgba(0, 0, 0, 0.06);
}

.back-btn:hover {
  background: rgba(255, 255, 255, 0.32);
  transform: translateY(-2px);
  box-shadow: 0 6px 20px rgba(0, 0, 0, 0.08);
}

.sync-btn {
  background: #2f855a;
  color: white;
  padding: 8px 14px;
  border: none;
  border-radius: 10px;
  cursor: pointer;
  font-size: 13px;
  font-weight: 600;
  box-shadow: 0 4px 12px rgba(103, 194, 58, 0.2);
  transition: all 0.2s ease;
}

.sync-btn:disabled {
  background: #ccc;
  box-shadow: none;
  cursor: not-allowed;
}

.settings-btn {
  padding: 8px 14px;
  border: 1px solid rgba(0, 0, 0, 0.1);
  border-radius: 8px;
  background: #fff;
  color: #2c3e50;
  cursor: pointer;
  font-weight: 600;
}

.settings-btn:hover {
  border-color: #409eff;
  color: #1677c8;
}

.capability-panel {
  display: grid;
  grid-template-columns: repeat(2, minmax(180px, 1fr));
  gap: 12px 16px;
  padding: 16px 24px;
  background: #fff;
  border-bottom: 1px solid rgba(0, 0, 0, 0.06);
}

.capability-field {
  min-width: 0;
  display: flex;
  flex-direction: column;
  gap: 6px;
}

.capability-field-wide,
.capability-tools,
.knowledge-status,
.capability-error {
  grid-column: 1 / -1;
}

.capability-field > label,
.capability-label {
  color: #2c3e50;
  font-size: 13px;
  font-weight: 600;
}

.capability-select {
  width: 100%;
  min-height: 36px;
  padding: 7px 9px;
  border: 1px solid rgba(0, 0, 0, 0.14);
  border-radius: 8px;
  background: #fff;
  color: #2c3e50;
}

.capability-select[multiple] {
  min-height: 72px;
}

.capability-select:disabled {
  background: #f4f5f7;
  color: #8995a2;
  cursor: not-allowed;
}

.capability-hint,
.knowledge-status {
  color: #667788;
  font-size: 12px;
  line-height: 1.5;
}

.knowledge-status {
  padding: 9px 11px;
  border: 1px solid #dce3e9;
  border-radius: 7px;
  background: #f7f9fb;
}

.knowledge-status.knowledge-error,
.capability-error {
  color: #b42318;
}

.capability-error {
  margin: 0;
  font-size: 13px;
}

.inline-action {
  margin-left: 8px;
  border: 0;
  background: transparent;
  color: #1677c8;
  cursor: pointer;
  text-decoration: underline;
}

.capability-tools {
  display: grid;
  gap: 8px;
}

.tool-group {
  padding: 8px 10px;
  border: 1px solid #dce3e9;
  border-radius: 7px;
  background: #fff;
}

.tool-group summary {
  cursor: pointer;
  color: #2c3e50;
  font-size: 13px;
  font-weight: 600;
}

.tool-option {
  display: inline-flex;
  align-items: center;
  gap: 6px;
  margin: 9px 14px 0 0;
  color: #4a5a68;
  font-size: 13px;
}

.upload-btn {
  background: #1677c8;
  color: white;
  padding: 8px 14px;
  border: none;
  border-radius: 10px;
  cursor: pointer;
  font-size: 13px;
  font-weight: 600;
  box-shadow: 0 2px 8px rgba(22, 119, 200, 0.2);
  transition: all 0.2s ease;
}

.upload-btn:hover:not(:disabled) {
  background: #125f9f;
}

.upload-btn:disabled {
  background: #ccc;
  box-shadow: none;
  cursor: not-allowed;
}

.chat-messages {
  flex: 1;
  min-height: 0;
  overflow-y: auto;
  padding: 30px;
  display: flex;
  flex-direction: column;
  gap: 18px;
  position: relative;
  z-index: 1;
}

/* scrollbar */
.chat-messages::-webkit-scrollbar {
  width: 8px;
}
.chat-messages::-webkit-scrollbar-thumb {
  background: rgba(0,0,0,0.12);
  border-radius: 8px;
}
.chat-messages::-webkit-scrollbar-track {
  background: transparent;
}

.message {
  max-width: 70%;
  padding: 14px 18px;
  border-radius: 18px;
  line-height: 1.6;
  word-wrap: break-word;
  position: relative;
  animation: messageSlideIn 0.28s ease-out;
  font-size: 15px;
  box-sizing: border-box;
}

@keyframes messageSlideIn {
  from {
    opacity: 0;
    transform: translateY(12px) scale(0.98);
  }
  to {
    opacity: 1;
    transform: translateY(0) scale(1);
  }
}

.user-message {
  align-self: flex-end;
  background: #e8f2fb;
  color: #16324f;
  border: 1px solid #cfe2f2;
  box-shadow: 0 2px 8px rgba(22, 54, 82, 0.06);
}

.user-message::after {
  content: '';
  position: absolute;
  bottom: -6px;
  right: 18px;
  width: 0;
  height: 0;
  border-left: 8px solid transparent;
  border-right: 8px solid transparent;
  border-top: 8px solid #e8f2fb;
}

.ai-message {
  align-self: flex-start;
  background: rgba(255, 255, 255, 0.95);
  backdrop-filter: blur(4px);
  color: #2c3e50;
  box-shadow: 0 6px 20px rgba(0, 0, 0, 0.06);
  border: 1px solid rgba(255, 255, 255, 0.3);
}

.ai-message::after {
  content: '';
  position: absolute;
  bottom: -6px;
  left: 18px;
  width: 0;
  height: 0;
  border-left: 8px solid transparent;
  border-right: 8px solid transparent;
  border-top: 8px solid rgba(255, 255, 255, 0.95);
}

.message-header {
  display: flex;
  align-items: center;
  gap: 10px;
  margin-bottom: 8px;
}

.message-header b {
  font-weight: 600;
}

.tts-btn {
  padding: 6px 10px;
  border-radius: 8px;
  font-size: 12px;
  cursor: pointer;
  background: #eaf6ef;
  color: #237342;
  border: 1px solid #bfe2ca;
  border: none;
  transition: all 0.18s ease;
  box-shadow: 0 2px 8px rgba(103, 194, 58, 0.18);
}

.tts-btn:hover {
  transform: scale(1.05);
  box-shadow: 0 4px 12px rgba(103, 194, 58, 0.25);
}

.streaming-indicator {
  color: #999;
  font-weight: 600;
  margin-left: 6px;
}

/* message content */
.message-content {
  white-space: pre-wrap;
  word-break: break-word;
}

/* input area */
.chat-input {
  padding: 24px;
  background: rgba(255, 255, 255, 0.96);
  backdrop-filter: blur(8px);
  border-top: 1px solid rgba(0, 0, 0, 0.06);
  position: relative;
  z-index: 1;
}

.chat-input textarea {
  width: 100%;
  resize: none;
  border: 2px solid rgba(0, 0, 0, 0.06);
  border-radius: 12px;
  padding: 14px 16px;
  font-size: 15px;
  outline: none;
  background: rgba(255,255,255,0.96);
  color: #2c3e50;
  transition: all 0.18s ease;
  min-height: 20px;
  max-height: 160px;
  box-shadow: 0 2px 10px rgba(0,0,0,0.04);
}

.chat-input textarea:focus {
  border-color: #409eff;
  box-shadow: 0 8px 30px rgba(64,158,255,0.06);
  transform: translateY(-1px);
}

.send-btn {
  position: absolute;
  right: 36px;
  bottom: 30px;
  padding: 12px 22px;
  border: none;
  border-radius: 50px;
  background: #1677c8;
  color: white;
  font-size: 15px;
  font-weight: 600;
  cursor: pointer;
  box-shadow: 0 3px 10px rgba(22,119,200,0.2);
  transition: all 0.18s ease;
}

.send-btn:hover:not(:disabled) {
  background: #125f9f;
}

.send-btn:disabled {
  background: #ccc;
  box-shadow: none;
  cursor: not-allowed;
}

@media (max-width: 820px) {
  .ai-chat-container {
    flex-direction: column;
  }

  .session-list {
    width: 100%;
    height: auto;
    max-height: 22vh;
  }

  .session-list-header {
    padding: 10px 14px;
    flex-direction: row;
    justify-content: space-between;
  }

  .new-chat-btn {
    width: auto;
    padding: 9px 16px;
  }

  .session-list-ul {
    display: flex;
    flex: 0 1 auto;
    overflow-x: auto;
    overflow-y: hidden;
  }

  .session-item {
    flex: 0 0 auto;
    padding: 10px 14px;
    white-space: nowrap;
  }

  .chat-section {
    flex: 1;
  }

  .top-bar {
    flex-wrap: wrap;
    padding: 10px 16px;
  }

  .capability-panel {
    grid-template-columns: 1fr;
    padding: 14px 16px;
  }

  .capability-field-wide,
  .capability-tools,
  .knowledge-status,
  .capability-error {
    grid-column: auto;
  }

  .chat-messages {
    padding: 18px;
  }

  .message {
    max-width: 88%;
  }

  .chat-input {
    padding: 16px;
  }

  .send-btn {
    right: 24px;
    bottom: 22px;
  }
}
</style>
