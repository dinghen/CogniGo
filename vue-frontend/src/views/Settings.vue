<template>
  <div class="settings-page">
    <header class="settings-header">
      <div>
        <p class="eyebrow">CogniGo</p>
        <h1>配置中心</h1>
        <p class="subtle">管理当前账号的模型连接、知识文件和 MCP 服务。</p>
      </div>
      <div class="header-actions">
        <el-button @click="$router.push('/ai-chat')">返回聊天</el-button>
        <el-button type="danger" plain @click="logout">退出登录</el-button>
      </div>
    </header>

    <main class="settings-content">
      <section class="settings-section">
        <div class="section-heading">
          <div>
            <h2>模型连接</h2>
            <p>API Key 只显示掩码，切换 Embedding 模型后需要重新构建索引。</p>
          </div>
          <el-button type="primary" :loading="savingProvider" @click="openProviderForm()">添加连接</el-button>
        </div>
        <el-alert v-if="providerError" type="error" :title="providerError" show-icon :closable="false" />
        <div v-loading="loadingProviders" class="resource-grid">
          <article v-for="provider in providers" :key="provider.id" class="resource-card">
            <div class="resource-card-top">
              <div>
                <span class="resource-kind">{{ provider.kind === 'embedding' ? 'Embedding' : '聊天模型' }}</span>
                <h3>{{ provider.name }}</h3>
              </div>
              <el-tag :type="statusType(provider.status)">{{ provider.status || 'unverified' }}</el-tag>
            </div>
            <p class="resource-meta">{{ provider.model }}</p>
            <p class="resource-meta truncate">{{ provider.base_url }}</p>
            <p class="resource-meta">Key: {{ provider.api_key_masked || '未配置' }}<span v-if="provider.embedding_dimension"> · {{ provider.embedding_dimension }} 维</span></p>
            <div class="card-actions">
              <el-button size="small" @click="testProvider(provider)">连接测试</el-button>
              <el-button v-if="provider.kind === 'embedding'" size="small" @click="rebuildProvider(provider)">重建索引</el-button>
              <el-button size="small" text @click="openProviderForm(provider)">编辑</el-button>
              <el-button size="small" text type="danger" @click="removeProvider(provider)">删除</el-button>
            </div>
          </article>
          <el-empty v-if="!loadingProviders && !providers.length" description="还没有模型连接" />
        </div>
      </section>

      <section class="settings-section">
        <div class="section-heading">
          <div>
            <h2>知识库</h2>
            <p>上传文件后会按当前 Embedding 配置异步建立 Redis 向量索引。</p>
          </div>
          <el-upload :show-file-list="false" :http-request="uploadKnowledge" accept=".md,.txt" :disabled="uploading">
            <el-button type="primary" :loading="uploading">上传文件</el-button>
          </el-upload>
        </div>
        <el-alert v-if="knowledgeError" type="error" :title="knowledgeError" show-icon :closable="false" />
        <div v-loading="loadingKnowledge" class="file-list">
          <div v-for="file in knowledgeFiles" :key="file.filename || file.name" class="file-row">
            <div>
              <strong>{{ file.filename || file.name }}</strong>
              <span>{{ file.status || 'ready' }}</span>
            </div>
            <el-button text type="danger" @click="removeKnowledge(file)">删除</el-button>
          </div>
          <el-empty v-if="!loadingKnowledge && !knowledgeFiles.length" description="还没有知识文件" />
        </div>
      </section>

      <section class="settings-section">
        <div class="section-heading">
          <div>
            <h2>MCP 服务</h2>
            <p>支持 Streamable HTTP 和受白名单控制的 stdio。连接测试会执行 Initialize 和 ListTools。</p>
          </div>
          <el-button type="primary" @click="openMcpForm()">添加服务</el-button>
        </div>
        <el-alert v-if="mcpError" type="error" :title="mcpError" show-icon :closable="false" />
        <div v-loading="loadingMcp" class="resource-grid">
          <article v-for="server in mcpServers" :key="server.id" class="resource-card">
            <div class="resource-card-top">
              <div>
                <span class="resource-kind">{{ server.transport }}</span>
                <h3>{{ server.name }}</h3>
              </div>
              <el-tag :type="statusType(server.status)">{{ server.status || 'unverified' }}</el-tag>
            </div>
            <p class="resource-meta truncate">{{ server.url || server.command }}</p>
            <p class="resource-meta">{{ server.tools?.length || 0 }} 个已发现工具</p>
            <div class="tool-chips">
              <el-tag v-for="tool in server.tools || []" :key="tool.id" size="small">{{ tool.name }}</el-tag>
            </div>
            <div class="card-actions">
              <el-button size="small" @click="testMcp(server)">测试并发现工具</el-button>
              <el-button size="small" text @click="openMcpForm(server)">编辑</el-button>
              <el-button size="small" text type="danger" @click="removeMcp(server)">删除</el-button>
            </div>
          </article>
          <el-empty v-if="!loadingMcp && !mcpServers.length" description="还没有 MCP 服务" />
        </div>
      </section>
    </main>

    <el-dialog v-model="providerDialog" :title="editingProvider ? '编辑模型连接' : '添加模型连接'" width="520px">
      <el-form :model="providerForm" label-position="top">
        <el-form-item label="名称"><el-input v-model="providerForm.name" placeholder="例如：我的聊天模型" /></el-form-item>
        <el-form-item label="类型"><el-select v-model="providerForm.kind" style="width: 100%"><el-option label="聊天模型" value="chat" /><el-option label="Embedding" value="embedding" /></el-select></el-form-item>
        <el-form-item label="OpenAI-compatible Base URL"><el-input v-model="providerForm.base_url" placeholder="https://.../v1" /></el-form-item>
        <el-form-item label="模型名称"><el-input v-model="providerForm.model" /></el-form-item>
        <el-form-item label="API Key"><el-input v-model="providerForm.api_key" type="password" show-password placeholder="编辑时留空表示保持原 Key" /></el-form-item>
        <el-checkbox v-model="providerForm.is_default">设为此类型默认连接</el-checkbox>
      </el-form>
      <template #footer><el-button @click="providerDialog = false">取消</el-button><el-button type="primary" :loading="savingProvider" @click="saveProvider">保存</el-button></template>
    </el-dialog>

    <el-dialog v-model="mcpDialog" :title="editingMcp ? '编辑 MCP 服务' : '添加 MCP 服务'" width="560px">
      <el-form :model="mcpForm" label-position="top">
        <el-form-item label="名称"><el-input v-model="mcpForm.name" placeholder="例如：天气服务" /></el-form-item>
        <el-form-item label="传输方式"><el-radio-group v-model="mcpForm.transport"><el-radio-button label="streamable-http">Streamable HTTP</el-radio-button><el-radio-button label="stdio">stdio</el-radio-button></el-radio-group></el-form-item>
        <el-form-item v-if="mcpForm.transport === 'streamable-http'" label="URL"><el-input v-model="mcpForm.url" placeholder="https://example.com/mcp" /></el-form-item>
        <el-form-item v-if="mcpForm.transport === 'streamable-http'" label="Headers（JSON）"><el-input v-model="mcpHeadersText" type="textarea" :rows="3" placeholder='{"Authorization":"Bearer ..."}' /></el-form-item>
        <template v-else>
          <el-alert type="warning" title="stdio 需要服务端开启 COGNIGO_MCP_ALLOW_STDIO 并配置命令白名单" :closable="false" />
          <el-form-item label="命令"><el-input v-model="mcpForm.command" placeholder="node" /></el-form-item>
          <el-form-item label="参数（每行一个）"><el-input v-model="mcpArgsText" type="textarea" :rows="3" /></el-form-item>
          <el-form-item label="环境变量（JSON）"><el-input v-model="mcpEnvText" type="textarea" :rows="3" /></el-form-item>
        </template>
      </el-form>
      <template #footer><el-button @click="mcpDialog = false">取消</el-button><el-button type="primary" :loading="savingMcp" @click="saveMcp">保存</el-button></template>
    </el-dialog>
  </div>
</template>

<script>
import { onMounted, reactive, ref } from 'vue'
import { ElMessage, ElMessageBox } from 'element-plus'
import { useRouter } from 'vue-router'
import api from '../utils/api'

const blankProvider = () => ({ name: '', kind: 'chat', protocol: 'openai-compatible', base_url: '', model: '', api_key: '', is_default: false })
const blankMcp = () => ({ name: '', transport: 'streamable-http', url: '', command: '', args: [], env: {}, headers: {}, enabled: true })

export default {
  name: 'SettingsView',
  setup() {
    const router = useRouter()
    const providers = ref([])
    const mcpServers = ref([])
    const knowledgeFiles = ref([])
    const loadingProviders = ref(false)
    const loadingMcp = ref(false)
    const loadingKnowledge = ref(false)
    const savingProvider = ref(false)
    const savingMcp = ref(false)
    const uploading = ref(false)
    const providerError = ref('')
    const mcpError = ref('')
    const knowledgeError = ref('')
    const providerDialog = ref(false)
    const mcpDialog = ref(false)
    const editingProvider = ref(null)
    const editingMcp = ref(null)
    const providerForm = reactive(blankProvider())
    const mcpForm = reactive(blankMcp())
    const mcpHeadersText = ref('{}')
    const mcpEnvText = ref('{}')
    const mcpArgsText = ref('')

    const statusType = status => ({ ready: 'success', healthy: 'success', rebuild_required: 'warning', failed: 'danger' }[status] || 'info')
    const loadProviders = async () => { loadingProviders.value = true; providerError.value = ''; try { const { data } = await api.get('/settings/providers'); providers.value = data.providers || [] } catch (e) { providerError.value = '模型连接加载失败' } finally { loadingProviders.value = false } }
    const loadMcp = async () => { loadingMcp.value = true; mcpError.value = ''; try { const { data } = await api.get('/settings/mcp-servers'); mcpServers.value = data.servers || [] } catch (e) { mcpError.value = 'MCP 服务加载失败' } finally { loadingMcp.value = false } }
    const loadKnowledge = async () => { loadingKnowledge.value = true; knowledgeError.value = ''; try { const { data } = await api.get('/file/files'); knowledgeFiles.value = data.files || [] } catch (e) { knowledgeError.value = '知识文件加载失败' } finally { loadingKnowledge.value = false } }
    const reload = () => Promise.all([loadProviders(), loadMcp(), loadKnowledge()])

    const openProviderForm = provider => { editingProvider.value = provider || null; Object.assign(providerForm, provider ? { ...provider, api_key: '' } : blankProvider()); providerDialog.value = true }
    const saveProvider = async () => { savingProvider.value = true; try { const payload = { ...providerForm }; if (!payload.api_key) delete payload.api_key; const request = editingProvider.value ? api.put(`/settings/providers/${editingProvider.value.id}`, payload) : api.post('/settings/providers', payload); await request; providerDialog.value = false; ElMessage.success('模型连接已保存'); await loadProviders() } catch (e) { ElMessage.error(e.response?.data?.status_msg || '保存模型连接失败') } finally { savingProvider.value = false } }
    const testProvider = async provider => { try { const { data } = await api.post(`/settings/providers/${provider.id}/test`); ElMessage.success(data.dimension ? `连接成功，向量维度 ${data.dimension}` : '连接成功'); await loadProviders() } catch (e) { ElMessage.error(e.response?.data?.status_msg || '连接测试失败') } }
    const rebuildProvider = async provider => { try { await ElMessageBox.confirm('重建会重新向量化当前账号的知识文件，继续吗？', '确认重建', { type: 'warning' }); await api.post(`/settings/providers/${provider.id}/rebuild`); ElMessage.success('索引重建已开始'); await loadProviders() } catch (e) { if (e !== 'cancel' && e !== 'close') ElMessage.error(e.response?.data?.status_msg || '索引重建失败') } }
    const removeProvider = async provider => { try { await ElMessageBox.confirm(`删除连接“${provider.name}”？`, '确认删除', { type: 'warning' }); await api.delete(`/settings/providers/${provider.id}`); await loadProviders() } catch (e) { if (e !== 'cancel' && e !== 'close') ElMessage.error('删除连接失败') } }

    const openMcpForm = server => { editingMcp.value = server || null; Object.assign(mcpForm, server ? { ...server, headers: {}, env: {}, args: server.args || [] } : blankMcp()); mcpHeadersText.value = '{}'; mcpEnvText.value = '{}'; mcpArgsText.value = (server?.args || []).join('\n'); mcpDialog.value = true }
    const parseObject = text => { const value = JSON.parse(text || '{}'); if (!value || Array.isArray(value) || typeof value !== 'object') throw new Error('必须是 JSON 对象'); return value }
    const saveMcp = async () => { savingMcp.value = true; try { const payload = { ...mcpForm, args: mcpForm.transport === 'stdio' ? mcpArgsText.value.split('\n').map(v => v.trim()).filter(Boolean) : [], headers: mcpForm.transport === 'streamable-http' ? parseObject(mcpHeadersText.value) : {}, env: mcpForm.transport === 'stdio' ? parseObject(mcpEnvText.value) : {} }; const request = editingMcp.value ? api.put(`/settings/mcp-servers/${editingMcp.value.id}`, payload) : api.post('/settings/mcp-servers', payload); await request; mcpDialog.value = false; ElMessage.success('MCP 服务已保存'); await loadMcp() } catch (e) { ElMessage.error(e.response?.data?.status_msg || e.message || '保存 MCP 服务失败') } finally { savingMcp.value = false } }
    const testMcp = async server => { try { await api.post(`/settings/mcp-servers/${server.id}/test`); ElMessage.success('连接成功，工具目录已更新'); await loadMcp() } catch (e) { ElMessage.error(e.response?.data?.status_msg || 'MCP 连接测试失败') } }
    const removeMcp = async server => { try { await ElMessageBox.confirm(`删除服务“${server.name}”？`, '确认删除', { type: 'warning' }); await api.delete(`/settings/mcp-servers/${server.id}`); await loadMcp() } catch (e) { if (e !== 'cancel' && e !== 'close') ElMessage.error('删除 MCP 服务失败') } }
    const uploadKnowledge = async options => { uploading.value = true; try { const form = new FormData(); form.append('file', options.file); await api.post('/file/upload', form, { headers: { 'Content-Type': 'multipart/form-data' } }); ElMessage.success('文件上传成功'); await loadKnowledge() } catch (e) { options.onError(e); ElMessage.error('文件上传失败') } finally { uploading.value = false } }
    const removeKnowledge = async file => { const name = file.filename || file.name; try { await ElMessageBox.confirm(`删除文件“${name}”？`, '确认删除', { type: 'warning' }); await api.delete(`/file/files/${encodeURIComponent(name)}`); await loadKnowledge() } catch (e) { if (e !== 'cancel' && e !== 'close') ElMessage.error('删除知识文件失败') } }
    const logout = () => { localStorage.removeItem('token'); router.push('/login') }

    onMounted(reload)
    return { providers, mcpServers, knowledgeFiles, loadingProviders, loadingMcp, loadingKnowledge, savingProvider, savingMcp, uploading, providerError, mcpError, knowledgeError, providerDialog, mcpDialog, editingProvider, editingMcp, providerForm, mcpForm, mcpHeadersText, mcpEnvText, mcpArgsText, statusType, openProviderForm, saveProvider, testProvider, rebuildProvider, removeProvider, openMcpForm, saveMcp, testMcp, removeMcp, uploadKnowledge, removeKnowledge, logout }
  }
}
</script>

<style scoped>
.settings-page { min-height: 100vh; background: #f4f6f8; color: #243142; }
.settings-header { display: flex; justify-content: space-between; gap: 24px; align-items: flex-start; padding: 42px max(24px, calc((100vw - 1180px) / 2)); background: #17212b; color: #fff; }
.eyebrow, .resource-kind { margin: 0 0 8px; color: #91b8d6; font-size: 12px; text-transform: uppercase; letter-spacing: 1px; }
h1, h2, h3, p { margin-top: 0; }
h1 { margin-bottom: 8px; font-size: 32px; }
.subtle, .section-heading p, .resource-meta { color: #667788; }
.settings-header .subtle { color: #c6d1da; margin-bottom: 0; }
.header-actions { display: flex; gap: 10px; }
.settings-content { max-width: 1180px; margin: 0 auto; padding: 32px 24px 64px; }
.settings-section { margin-bottom: 38px; }
.section-heading { display: flex; justify-content: space-between; align-items: flex-end; gap: 20px; margin-bottom: 18px; }
.section-heading h2 { margin-bottom: 7px; font-size: 21px; }
.section-heading p { margin-bottom: 0; font-size: 14px; }
.resource-grid { display: grid; grid-template-columns: repeat(auto-fit, minmax(280px, 1fr)); gap: 14px; min-height: 100px; }
.resource-card { background: #fff; border: 1px solid #dce3e9; border-radius: 8px; padding: 20px; box-shadow: 0 2px 8px rgb(23 33 43 / 5%); }
.resource-card-top { display: flex; justify-content: space-between; gap: 12px; align-items: flex-start; }
.resource-card h3 { margin-bottom: 14px; font-size: 17px; }
.resource-meta { margin-bottom: 8px; font-size: 13px; }
.truncate { overflow: hidden; text-overflow: ellipsis; white-space: nowrap; }
.card-actions { display: flex; flex-wrap: wrap; gap: 4px; margin-top: 16px; }
.tool-chips { display: flex; flex-wrap: wrap; gap: 6px; min-height: 24px; }
.file-list { overflow: hidden; background: #fff; border: 1px solid #dce3e9; border-radius: 8px; }
.file-row { display: flex; justify-content: space-between; align-items: center; padding: 14px 18px; border-bottom: 1px solid #edf0f2; }
.file-row:last-child { border-bottom: 0; }
.file-row strong { display: block; margin-bottom: 5px; }
.file-row span { color: #72808d; font-size: 13px; }
@media (max-width: 720px) { .settings-header, .section-heading { flex-direction: column; align-items: stretch; } .header-actions { justify-content: flex-start; } .settings-content { padding: 24px 16px 48px; } }
</style>
